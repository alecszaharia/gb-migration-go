package global_blocks

// Fault-injection tests (T-021): full parallel migrations (K=4) aborted at
// every hook point, then the committed state is checked against the
// per-range watermark invariants. Criteria RS R4 c4, c5 and PE R4 c2.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	crashK     = 4
	crashBatch = 3
)

// seedCrashFixture seeds 40 eligible blocks (5 of them failing, one of each
// kind in rotation) interleaved with ineligible rows. The K=4 split therefore
// has 10 eligible IDs per range: 4 batches of -b 3 (3, 3, 3, 1).
func seedCrashFixture(fx *fixture) {
	fx.t.Helper()
	kinds := []failKind{failMalformedRules, failInvalidIRI, failMissingCollectionType}
	for i := 1; i <= 40; i++ {
		if i%9 == 4 {
			fx.addFailing(kinds[(i/9)%len(kinds)])
		} else {
			fx.addEligible()
		}
		if i%13 == 0 {
			fx.addWrongNode()
		}
		if i%17 == 0 {
			fx.addLegacyAPI()
		}
	}
}

// crashRunArgs are the command args of a full K=4 run with batch 3.
func crashRunArgs() []string {
	return []string{"-w", fmt.Sprint(crashK), "-b", fmt.Sprint(crashBatch)}
}

// idsInRange returns the IDs of ids (ascending) that fall inside r.
func idsInRange(ids []int64, r idRange) []int64 {
	var out []int64
	for _, id := range ids {
		if r.contains(id) {
			out = append(out, id)
		}
	}
	return out
}

// loadStoredSplit reads the stored split from the fixture schema.
func loadStoredSplit(t *testing.T, fx *fixture) ([]storedRange, bool) {
	t.Helper()
	ctx := context.Background()
	st := &state{}
	if err := st.init(ctx, fx.DB); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	_, ranges, ok, err := st.loadSplit(ctx)
	if err != nil {
		t.Fatalf("loadSplit: %v", err)
	}
	return ranges, ok
}

// countOrphans returns the number of target rows not reachable from a
// global_block row, per table.
func countOrphans(t *testing.T, fx *fixture) map[string]int64 {
	t.Helper()
	q := map[string]string{
		"page_data":     `SELECT COUNT(*) FROM page_data pd LEFT JOIN global_block gb ON gb.page_data_id = pd.id WHERE gb.id IS NULL`,
		"compiled_data": `SELECT COUNT(*) FROM compiled_data cd LEFT JOIN global_block gb ON gb.compiled_data_id = cd.id WHERE gb.id IS NULL`,
		"rules":         `SELECT COUNT(*) FROM rules r LEFT JOIN global_block gb ON gb.id = r.global_block WHERE gb.id IS NULL`,
	}
	out := map[string]int64{}
	for table, query := range q {
		if n := mustInt(t, queryRows(t, fx.DB, query)[0][0]); n != 0 {
			out[table] = n
		}
	}
	return out
}

// assertWatermarkInvariants checks the committed state after an abort, for
// every stored range:
//   - every eligible ID <= watermark has migrated data or a failed row (RS R4 c4);
//   - no eligible ID > watermark has migrated data or a failed row (RS R4 c5);
//   - so the processed set of each range is exactly the eligible IDs up to
//     its watermark: no batch is half-present (PE R4 c2).
//
// It also checks that nothing was migrated twice or into the wrong table and
// that no target row is orphaned. It returns the stored ranges (nil when no
// split was stored).
func assertWatermarkInvariants(t *testing.T, fx *fixture) []storedRange {
	t.Helper()
	migrated := migratedDataIDs(t, fx.DB)
	failed := failedIDs(t, fx.DB)

	ranges, ok := loadStoredSplit(t, fx)
	if !ok {
		if len(migrated) != 0 || len(failed) != 0 {
			t.Errorf("no stored split but migrated=%v failed=%v", migrated, failed)
		}
		return nil
	}

	okIDs, failingIDs := fx.okIDs(), fx.failingIDs()
	eligible := fx.eligibleIDs()

	counts := map[int64]int{}
	for _, id := range migrated {
		counts[id]++
		if counts[id] == 2 {
			t.Errorf("block %d migrated more than once", id)
		}
		if !slices.Contains(okIDs, id) {
			t.Errorf("migrated block %d is not an ok eligible block", id)
		}
	}
	for _, id := range failed {
		if !slices.Contains(failingIDs, id) {
			t.Errorf("failed row for %d, which is not a failing block", id)
		}
		if counts[id] > 0 {
			t.Errorf("block %d has both migrated data and a failed row", id)
		}
	}

	processed := slices.Concat(slices.Compact(slices.Clone(migrated)), failed)
	slices.Sort(processed)

	covered := 0
	for _, r := range ranges {
		inRange := idsInRange(eligible, r.idRange)
		var want []int64
		for _, id := range inRange {
			if id <= r.Watermark {
				want = append(want, id)
			}
		}
		// the watermark is either the initial one or a committed batch max
		if r.Watermark != r.Lower-1 && !slices.Contains(inRange, r.Watermark) {
			t.Errorf("range %d watermark %d is neither lower-1 nor an eligible ID of the range", r.Index, r.Watermark)
		}
		got := idsInRange(processed, r.idRange)
		covered += len(got)
		if d := diffIDs(got, want); d != "" {
			t.Errorf("range %d [%d, %v] watermark %d: processed IDs != eligible IDs <= watermark: %s",
				r.Index, r.Lower, derefUpper(r.Upper), r.Watermark, d)
		}
	}
	if covered != len(processed) {
		t.Errorf("%d processed IDs fall outside every stored range: %v", len(processed)-covered, processed)
	}

	if o := countOrphans(t, fx); len(o) != 0 {
		t.Errorf("orphan target rows: %v", o)
	}
	var wantRules int64
	for _, id := range migrated {
		wantRules += int64(fx.block(id).RuleCount)
	}
	rc := rowCounts(t, fx.DB)
	n := int64(len(migrated))
	wantCounts := map[string]int64{"global_block": n, "page_data": n, "compiled_data": n, "rules": wantRules}
	for table, want := range wantCounts {
		if rc[table] != want {
			t.Errorf("%s has %d rows, want %d", table, rc[table], want)
		}
	}
	return ranges
}

// crashPoint is where a run is aborted.
type crashPoint int

const (
	crashAtBatchStart   crashPoint = iota // testHookBatchStart
	crashAfter1Block                      // mid-batch, after 1 savepoint
	crashAfter2Blocks                     // mid-batch, after 2 savepoints
	crashAtBeforeCommit                   // testHookBeforeBatchCommit
)

func (p crashPoint) String() string {
	return [...]string{"batch_start", "after_1_block", "after_2_blocks", "before_commit"}[p]
}

// crashMode is how a run is aborted.
type crashMode int

const (
	crashReturnError crashMode = iota // the hook returns an error
	crashCancelCtx                    // the hook cancels the run's context
)

func (m crashMode) String() string {
	return [...]string{"error", "cancel"}[m]
}

// crashCase aborts at the occ-th (1-based) occurrence of point in range rangeIdx.
type crashCase struct {
	point    crashPoint
	mode     crashMode
	rangeIdx int
	occ      int
}

// resetMigrationOutput deletes everything the tool writes (migrated tables,
// failed table, state table) so a seeded fixture can be migrated again from
// scratch. Seeded source rows are untouched.
func resetMigrationOutput(t *testing.T, fx *fixture) {
	t.Helper()
	for _, table := range []string{"rules", "global_block", "page_data", "compiled_data", "global_block_migration_failed", stateTable} {
		if !tableExists(t, fx.DB, table) {
			continue
		}
		if _, err := fx.DB.ExecContext(context.Background(), "DELETE FROM `"+table+"`"); err != nil {
			t.Fatalf("reset %s: %v", table, err)
		}
	}
}

// runCrashCase resets the seeded fixture base, runs the full K=4 migration
// with the fault installed, and checks the invariants afterwards.
func runCrashCase(t *testing.T, base *fixture, c crashCase) {
	fx := *base
	fx.t = t
	resetMigrationOutput(t, &fx)
	if n := len(migratedDataIDs(t, fx.DB)) + len(failedIDs(t, fx.DB)) + len(stateRows(t, fx.DB)); n != 0 {
		t.Fatalf("reset left %d rows", n)
	}
	split := computeSplit(fx.eligibleIDs(), crashK)
	rangeOf := func(id int64) int {
		for _, r := range split {
			if r.contains(id) {
				return r.Index
			}
		}
		t.Errorf("block %d is in no range", id)
		return -1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	injected := errors.New("injected crash")

	var (
		mu    sync.Mutex
		seen  int
		fired bool
	)
	trigger := func(rangeIdx int) error {
		mu.Lock()
		defer mu.Unlock()
		if rangeIdx != c.rangeIdx || fired {
			return nil
		}
		seen++
		if seen != c.occ {
			return nil
		}
		fired = true
		if c.mode == crashCancelCtx {
			cancel()
			return nil
		}
		return injected
	}

	switch c.point {
	case crashAtBatchStart:
		setTestHook(t, &testHookBatchStart, trigger)
	case crashAtBeforeCommit:
		setTestHook(t, &testHookBeforeBatchCommit, trigger)
	case crashAfter1Block, crashAfter2Blocks:
		n := 1
		if c.point == crashAfter2Blocks {
			n = 2
		}
		setTestHook(t, &testHookBeforeBlock, func(dataID int64, done int) error {
			if done != n {
				return nil
			}
			return trigger(rangeOf(dataID))
		})
	}

	_, err := fx.run(ctx, crashRunArgs()...)
	if !fired {
		t.Fatalf("fault never fired (run err: %v)", err)
	}
	if err == nil {
		t.Fatal("run succeeded, want an error after the fault")
	}
	// With an injected error the other workers' cancellations are not
	// reported, so the command's error is the injected one.
	if c.mode == crashReturnError && !errors.Is(err, injected) {
		t.Errorf("run error = %v, want the injected error", err)
	}

	ranges := assertWatermarkInvariants(t, &fx)
	if len(ranges) != crashK {
		t.Fatalf("stored split has %d ranges, want %d", len(ranges), crashK)
	}

	// The faulted range committed exactly occ-1 batches: the aborted batch
	// (or, at batch start, the batch about to begin) left no trace.
	inRange := idsInRange(fx.eligibleIDs(), ranges[c.rangeIdx].idRange)
	wantWM := ranges[c.rangeIdx].Lower - 1
	if done := (c.occ - 1) * crashBatch; done > 0 {
		wantWM = inRange[done-1]
	}
	if got := ranges[c.rangeIdx].Watermark; got != wantWM {
		t.Errorf("faulted range %d watermark = %d, want %d (%d batches committed)", c.rangeIdx, got, wantWM, c.occ-1)
	}
}

// TestCrashKeepsWatermarkInvariants aborts full K=4 runs at every hook point,
// by error and by cancellation, at the 1st, 2nd and 3rd occurrence on
// different ranges (including the open-ended last one).
func TestCrashKeepsWatermarkInvariants(t *testing.T) {
	base := newFixture(t)
	seedCrashFixture(base)
	where := []struct{ rangeIdx, occ int }{{0, 1}, {1, 2}, {2, 2}, {3, 3}}
	for _, point := range []crashPoint{crashAtBatchStart, crashAfter1Block, crashAfter2Blocks, crashAtBeforeCommit} {
		for _, mode := range []crashMode{crashReturnError, crashCancelCtx} {
			for _, w := range where {
				c := crashCase{point: point, mode: mode, rangeIdx: w.rangeIdx, occ: w.occ}
				t.Run(fmt.Sprintf("%s/%s/range%d/occ%d", point, mode, w.rangeIdx, w.occ), func(t *testing.T) {
					runCrashCase(t, base, c)
				})
			}
		}
	}
}

// TestCrashSIGKILL builds the real binary, kills it with SIGKILL at random
// short delays (resuming each time from the stored state), checks the
// invariants after every kill, and finally runs to completion.
func TestCrashSIGKILL(t *testing.T) {
	fx := newFixture(t)
	kinds := []failKind{failMalformedRules, failInvalidIRI, failMissingCollectionType}
	for i := 1; i <= 300; i++ {
		if i%23 == 0 {
			fx.addFailing(kinds[(i/23)%len(kinds)])
		} else {
			fx.addEligible()
		}
	}

	bin := filepath.Join(t.TempDir(), "gbmigrate")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join("..", "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	seed := time.Now().UnixNano()
	t.Logf("kill delay seed %d", seed)
	rng := rand.New(rand.NewPCG(uint64(seed), 0))

	kills := 0
	for attempt := 0; attempt < 6; attempt++ {
		cmd := exec.Command(bin, "global_blocks", "-d", fx.DSN, "-w", "4", "-b", "5")
		cmd.Stdout, cmd.Stderr = nil, nil
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		delay := time.Duration(20+rng.IntN(200)) * time.Millisecond
		select {
		case err := <-done:
			t.Logf("attempt %d: finished before the %v kill (err %v)", attempt, delay, err)
		case <-time.After(delay):
			if err := cmd.Process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Fatalf("kill: %v", err)
			}
			<-done
			kills++
			t.Logf("attempt %d: killed after %v", attempt, delay)
		}
		assertWatermarkInvariants(t, fx)
		if t.Failed() {
			t.FailNow()
		}
	}
	if kills == 0 {
		t.Log("note: every run finished before its kill; only the completed-run invariants were exercised")
	}

	if out, err := fx.run(context.Background(), "-w", "4", "-b", "5"); err != nil {
		t.Fatalf("final run: %v\n%s", err, out)
	}
	assertWatermarkInvariants(t, fx)
	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("final migrated: %s", d)
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Errorf("final failed: %s", d)
	}
}
