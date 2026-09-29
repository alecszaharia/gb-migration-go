package global_blocks

// Integration tests of fatal error propagation and reporting (T-024).
// Criteria PE R4 c1, c3, c4, c6.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// feSeed seeds 32 contiguous eligible blocks so a K=4 split is
// [1, 8], [9, 16], [17, 24], [25, +inf): four batches of 2 per range.
func feSeed(fx *fixture) { fx.addEligible(seqIDs(1, 32)...) }

// feBounds formats a range's bounds the way rangeError does.
func feBounds(r storedRange) string {
	upper := "+inf"
	if r.Upper != nil {
		upper = fmt.Sprint(*r.Upper)
	}
	return fmt.Sprintf("range %d [%d, %s]: ", r.Index, r.Lower, upper)
}

// TestFatalErrorStopsOtherWorkers: range 2 fails fatally before its second
// batch commits while every other worker holds its own second batch open.
// The other workers' open batches roll back and none of them starts another
// batch after the failure (PE R4 c1). The returned error names range 2 with
// its bounds (PE R4 c4).
func TestFatalErrorStopsOtherWorkers(t *testing.T) {
	const k, failRange, failBatch = 4, 2, 2
	fx := newFixture(t)
	feSeed(fx)
	e := newOrchEnv(t, fx.DB, k)

	var (
		mu      sync.Mutex
		starts  = map[int][]time.Time{}
		commits = map[int]int{}
		failAt  time.Time
		errs    []error
	)
	others := sync.WaitGroup{}
	others.Add(k - 1)
	failed := make(chan struct{})
	injected := errors.New("injected fatal error")
	// Long enough for the failing worker to return and the group context to
	// be cancelled before the held batches try to commit.
	const grace = 300 * time.Millisecond

	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		mu.Lock()
		starts[rangeIdx] = append(starts[rangeIdx], time.Now())
		mu.Unlock()
		return nil
	})
	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
		mu.Lock()
		commits[rangeIdx]++
		n := commits[rangeIdx]
		mu.Unlock()
		if n != failBatch {
			return nil
		}
		if rangeIdx != failRange {
			// hold this worker's second batch transaction open until the
			// failure has happened
			others.Done()
			select {
			case <-failed:
			case <-time.After(10 * time.Second):
				mu.Lock()
				errs = append(errs, fmt.Errorf("range %d: failure never happened", rangeIdx))
				mu.Unlock()
			}
			time.Sleep(grace)
			return nil
		}
		// fail once every other worker holds its second batch open
		done := make(chan struct{})
		go func() { others.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			mu.Lock()
			errs = append(errs, errors.New("other workers never reached their second batch"))
			mu.Unlock()
		}
		mu.Lock()
		failAt = time.Now()
		mu.Unlock()
		close(failed)
		return injected
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := runRanges(ctx, e.migSt, e.repo, e.ranges, 2, nil)
	for _, err := range errs {
		t.Error(err)
	}
	if !errors.Is(err, injected) {
		t.Fatalf("runRanges err = %v, want the injected error", err)
	}
	if want := feBounds(e.ranges[failRange]); !strings.HasPrefix(err.Error(), want) || want != "range 2 [17, 24]: " {
		t.Errorf("error = %q, want prefix %q (range 2 [17, 24])", err, want)
	}

	// PE R4 c1: no batch started after the failure. Every worker started
	// exactly its first two batches, all before the failure, although each
	// range had two more batches to go.
	for i := range e.ranges {
		if len(starts[i]) != 2 {
			t.Errorf("range %d: %d batch starts, want 2", i, len(starts[i]))
		}
		for j, ts := range starts[i] {
			if !ts.Before(failAt) {
				t.Errorf("range %d batch start %d at %v is not before the failure at %v", i, j+1, ts, failAt)
			}
		}
		if commits[i] != 2 {
			t.Errorf("range %d: %d pre-commit hook calls, want 2", i, commits[i])
		}
	}

	// Only first batches committed: the held second batches rolled back.
	stm := stateMap(t, fx.DB)
	var want []int64
	for i, r := range e.ranges {
		if got := stm[rangeWatermarkKey(i)]; got != r.Lower+1 {
			t.Errorf("range %d watermark = %d, want first batch max %d", i, got, r.Lower+1)
		}
		want = append(want, r.Lower, r.Lower+1)
	}
	if got := migratedDataIDs(t, fx.DB); !reflect.DeepEqual(got, want) {
		t.Errorf("migrated = %v, want only first batches %v", got, want)
	}
}

// TestFatalErrorCommandReportsRange: in-process command run; RunE returns the
// error naming the failing range and its bounds, "+inf" for the last range
// (PE R4 c3, c4).
func TestFatalErrorCommandReportsRange(t *testing.T) {
	for _, tc := range []struct {
		failRange int
		want      string
	}{
		{2, "range 2 [17, 24]: "},
		{3, "range 3 [25, +inf]: "},
	} {
		t.Run(fmt.Sprintf("range%d", tc.failRange), func(t *testing.T) {
			fx := newFixture(t)
			feSeed(fx)

			injected := errors.New("injected fatal error")
			var mu sync.Mutex
			calls := 0
			setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
				if rangeIdx != tc.failRange {
					return nil
				}
				mu.Lock()
				defer mu.Unlock()
				if calls++; calls == 2 {
					return injected
				}
				return nil
			})

			out, err := fx.run(context.Background(), "-b", "2", "-w", "4")
			if err == nil {
				t.Fatalf("run succeeded, want error\n%s", out)
			}
			if !errors.Is(err, injected) {
				t.Errorf("error %q does not wrap the injected error", err)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("error = %q, want prefix %q", err, tc.want)
			}
			if !strings.Contains(out, "Error: "+tc.want) {
				t.Errorf("output lacks %q:\n%s", "Error: "+tc.want, out)
			}
			// the failing range's first batch stays committed
			lower := stateMap(t, fx.DB)[rangeLowerKey(tc.failRange)]
			if wm := stateMap(t, fx.DB)[rangeWatermarkKey(tc.failRange)]; wm != lower+1 {
				t.Errorf("range %d watermark = %d, want %d", tc.failRange, wm, lower+1)
			}
		})
	}
}

// buildTool builds the command binary (module root main.go) once per test.
func buildTool(t *testing.T) string {
	t.Helper()
	root, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "gbm")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = strings.TrimSpace(string(root))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// TestFatalErrorSubprocessExitStatus runs the real binary. A MySQL trigger
// makes the watermark update of the failing range's second batch fail, a
// real fatal (non per-block) error. The process exits with status 1 and
// prints the range with its bounds (PE R4 c3, c4).
func TestFatalErrorSubprocessExitStatus(t *testing.T) {
	if os.Getenv(testDSNEnv) == "" {
		t.Skipf("%s not set; skipping MySQL integration test", testDSNEnv)
	}
	bin := buildTool(t)
	for _, tc := range []struct {
		failRange int
		want      string
	}{
		{2, "range 2 [17, 24]: "},
		{3, "range 3 [25, +inf]: "},
	} {
		t.Run(fmt.Sprintf("range%d", tc.failRange), func(t *testing.T) {
			fx := newFixture(t)
			feSeed(fx)

			// Same DDL as state.init; the trigger needs the table up front.
			fx.exec(`CREATE TABLE IF NOT EXISTS global_block_migration_state (
				set_key VARCHAR(64) NOT NULL PRIMARY KEY,
				value BIGINT NOT NULL DEFAULT 0
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
			// Initial watermarks are plain inserts; batch commits update them.
			// Batch 1 of the range sets lower+1; batch 2 trips the trigger.
			lower := int64(17)
			if tc.failRange == 3 {
				lower = 25
			}
			fx.exec(fmt.Sprintf(`CREATE TRIGGER gbtest_fail_watermark BEFORE UPDATE ON global_block_migration_state
				FOR EACH ROW BEGIN
					IF NEW.set_key = '%s' AND NEW.value > %d THEN
						SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected watermark failure';
					END IF;
				END`, rangeWatermarkKey(tc.failRange), lower+1))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "global_blocks", "-d", fx.DSN, "-w", "4", "-b", "2")
			out, err := cmd.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("run: err = %v, want a non-zero exit\n%s", err, out)
			}
			if code := exitErr.ExitCode(); code != 1 {
				t.Errorf("exit status = %d, want 1\n%s", code, out)
			}
			if !strings.Contains(string(out), "Error: "+tc.want) {
				t.Errorf("output lacks %q:\n%s", "Error: "+tc.want, out)
			}
			if !strings.Contains(string(out), "injected watermark failure") {
				t.Errorf("output lacks the underlying error:\n%s", out)
			}
			if wm := stateMap(t, fx.DB)[rangeWatermarkKey(tc.failRange)]; wm != lower+1 {
				t.Errorf("range %d watermark = %d, want %d", tc.failRange, wm, lower+1)
			}
		})
	}
}

// TestPerBlockFailureDoesNotCancel: failing blocks in every range are
// recorded in the failed table without cancelling any worker; all ranges
// run to completion (PE R4 c6).
func TestPerBlockFailureDoesNotCancel(t *testing.T) {
	fx := newFixture(t)
	fx.addEligible(1, 2)
	fx.addFailing(failMalformedRules, 3)
	fx.addEligible(4, 5, 6)
	fx.addFailing(failInvalidIRI, 7)
	fx.addEligible(8, 9, 10, 11)
	fx.addFailing(failMissingCollectionType, 12)
	fx.addEligible(13, 14, 15, 16, 17)
	fx.addFailing(failMalformedRules, 18)
	fx.addEligible(19, 20)
	// K=4 over 20 IDs: [1, 5], [6, 10], [11, 15], [16, +inf); every range
	// has a failing block.

	var mu sync.Mutex
	starts := map[int]int{}
	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		mu.Lock()
		starts[rangeIdx]++
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := fx.run(ctx, "-b", "2", "-w", "4")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}

	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("migrated IDs: %s", d)
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Errorf("failed IDs: %s", d)
	}
	stm := stateMap(t, fx.DB)
	eligible := fx.eligibleIDs()
	uppers := []int64{5, 10, 15}
	for i := 0; i < 4; i++ {
		lower := stm[rangeLowerKey(i)]
		var maxID int64
		n := 0
		for _, id := range eligible {
			if id >= lower && (i == 3 || id <= uppers[i]) {
				maxID = max(maxID, id)
				n++
			}
		}
		if got := stm[rangeWatermarkKey(i)]; got != maxID {
			t.Errorf("range %d watermark = %d, want its max eligible ID %d", i, got, maxID)
		}
		// 5 blocks, batch 2: 3 batches plus the final empty lookup
		if wantStarts := (n+1)/2 + 1; starts[i] != wantStarts {
			t.Errorf("range %d: %d batch starts, want %d", i, starts[i], wantStarts)
		}
	}
	for i, u := range uppers {
		if got := stm[rangeUpperKey(i)]; got != u {
			t.Errorf("range %d upper = %d, want %d", i, got, u)
		}
	}
}
