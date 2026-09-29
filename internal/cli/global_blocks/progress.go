package global_blocks

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// progressLinePrefix moves the cursor to the start of the previous line and
// clears it, so every render rewrites the same terminal line.
const progressLinePrefix = "\033[F\033[2K"

// progress is an aggregate progress reporter shared by all workers.
//
// Workers only call add; they never print. A single goroutine started by
// start renders the line on a ticker, and stop performs one final render.
type progress struct {
	total     int64
	processed atomic.Int64
	start     time.Time
	w         io.Writer
	now       func() time.Time

	mu      sync.Mutex // serialises writes to w
	stopCh  chan struct{}
	done    chan struct{}
	stopped sync.Once
}

// newProgress creates a reporter for total blocks. A nil w defaults to
// os.Stdout and a nil now defaults to time.Now.
func newProgress(total int64, w io.Writer, now func() time.Time) *progress {
	if w == nil {
		w = os.Stdout
	}
	if now == nil {
		now = time.Now
	}
	return &progress{
		total: total,
		start: now(),
		w:     w,
		now:   now,
	}
}

// add records n more processed blocks. Safe for concurrent use.
func (p *progress) add(n int64) {
	p.processed.Add(n)
}

// render writes the progress line, overwriting the previous one.
func (p *progress) render() {
	processed := p.processed.Load()

	percent := 100.0
	if p.total > 0 {
		percent = float64(processed) / float64(p.total) * 100
	}

	rate := 0.0
	if elapsed := p.now().Sub(p.start).Seconds(); elapsed > 0 {
		rate = float64(processed) / elapsed
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = fmt.Fprintf(p.w, progressLinePrefix+"Progress: %.2f%% (%d/%d) | %.2f blocks/s\n",
		percent, processed, p.total, rate)
}

// startRendering reserves the progress line and launches the single render
// goroutine, which renders every interval until stop is called.
func (p *progress) startRendering(interval time.Duration) {
	p.stopCh = make(chan struct{})
	p.done = make(chan struct{})

	p.mu.Lock()
	_, _ = fmt.Fprintln(p.w) // reserve the line the first render rewrites
	p.mu.Unlock()

	go func() {
		defer close(p.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.render()
			case <-p.stopCh:
				return
			}
		}
	}()
}

// stop halts the render goroutine, waits for it to exit and performs the
// final render. Calling stop more than once is a no-op.
func (p *progress) stop() {
	p.stopped.Do(func() {
		if p.stopCh != nil {
			close(p.stopCh)
			<-p.done
		}
		p.render()
	})
}
