package global_blocks

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a concurrency-safe, manually advanced clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// safeBuffer is a bytes.Buffer guarded by a mutex for concurrent reads.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestProgressRender(t *testing.T) {
	tests := []struct {
		name      string
		total     int64
		processed int64
		elapsed   time.Duration
		want      string
	}{
		{
			name:      "half done",
			total:     200,
			processed: 100,
			elapsed:   4 * time.Second,
			want:      "\033[F\033[2KProgress: 50.00% (100/200) | 25.00 blocks/s\n",
		},
		{
			name:      "fractional",
			total:     3,
			processed: 1,
			elapsed:   3 * time.Second,
			want:      "\033[F\033[2KProgress: 33.33% (1/3) | 0.33 blocks/s\n",
		},
		{
			name:      "no elapsed time",
			total:     10,
			processed: 0,
			elapsed:   0,
			want:      "\033[F\033[2KProgress: 0.00% (0/10) | 0.00 blocks/s\n",
		},
		{
			name:      "zero total",
			total:     0,
			processed: 0,
			elapsed:   time.Second,
			want:      "\033[F\033[2KProgress: 100.00% (0/0) | 0.00 blocks/s\n",
		},
		{
			name:      "processed exceeds total",
			total:     10,
			processed: 12,
			elapsed:   2 * time.Second,
			want:      "\033[F\033[2KProgress: 120.00% (12/10) | 6.00 blocks/s\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeClock()
			var buf bytes.Buffer
			p := newProgress(tt.total, &buf, clock.now)
			p.add(tt.processed)
			clock.advance(tt.elapsed)

			p.render()

			if got := buf.String(); got != tt.want {
				t.Errorf("render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProgressConcurrentAddSingleLine(t *testing.T) {
	const (
		goroutines = 16
		perWorker  = 1000
		total      = goroutines * perWorker
	)

	clock := newFakeClock()
	buf := &safeBuffer{}
	p := newProgress(total, buf, clock.now)
	p.startRendering(time.Millisecond)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				p.add(1)
			}
		}()
	}
	wg.Wait()
	clock.advance(10 * time.Second)
	p.stop()
	p.stop() // idempotent: no extra render

	out := buf.String()
	if !strings.HasPrefix(out, "\n") {
		t.Fatalf("output must start with the reserved line, got %q", out)
	}

	// After the reserved newline every line is a render that first moves up
	// and clears the previous line, so the terminal only ever shows one line.
	lines := strings.Split(strings.TrimSuffix(out[1:], "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, progressLinePrefix+"Progress: ") {
			t.Fatalf("line %d does not rewrite the progress line: %q", i, line)
		}
		if strings.Count(line, "Progress:") != 1 {
			t.Fatalf("line %d contains more than one progress entry: %q", i, line)
		}
	}

	final := lines[len(lines)-1]
	want := progressLinePrefix + "Progress: 100.00% (16000/16000) | 1600.00 blocks/s"
	if final != want {
		t.Errorf("final render = %q, want %q", final, want)
	}
	if got := p.processed.Load(); got != total {
		t.Errorf("processed = %d, want %d", got, total)
	}
}

func TestProgressStopWithoutStart(t *testing.T) {
	clock := newFakeClock()
	var buf bytes.Buffer
	p := newProgress(5, &buf, clock.now)
	p.add(5)
	clock.advance(time.Second)
	p.stop()

	want := progressLinePrefix + "Progress: 100.00% (5/5) | 5.00 blocks/s\n"
	if got := buf.String(); got != want {
		t.Errorf("stop() output = %q, want %q", got, want)
	}
}
