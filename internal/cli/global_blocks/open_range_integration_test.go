package global_blocks

// Integration tests of the open-ended last range (T-022). Criteria RS R3
// c1–c4.

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"
)

// orchEnv is a stored split plus the migration state and repository a
// worker needs, all built on db.
type orchEnv struct {
	db     *sql.DB
	migSt  *state
	repo   *repository
	ranges []storedRange
}

// newOrchEnv prepares the repository and state on db and stores (or loads)
// the split for k workers.
func newOrchEnv(t *testing.T, db *sql.DB, k int) *orchEnv {
	t.Helper()
	ctx := context.Background()
	repo, err := newPrepareRepository(ctx, db)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	migSt := &state{}
	if err := migSt.init(ctx, db); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	ranges, err := ensureSplit(ctx, db, migSt, repo, k)
	if err != nil {
		t.Fatalf("ensureSplit: %v", err)
	}
	if len(ranges) != k {
		t.Fatalf("split has %d ranges, want %d: %+v", len(ranges), k, ranges)
	}
	return &orchEnv{db: db, migSt: migSt, repo: repo, ranges: ranges}
}

// splitOnly returns the state keys that describe the split (split_k, lower
// and upper bounds), without the watermarks.
func splitOnly(st map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range st {
		if m := rangeKeyRe.FindStringSubmatch(k); (m != nil && m[2] != "watermark") || k == splitKKey {
			out[k] = v
		}
	}
	return out
}

// openRangeSeed seeds sparse eligible IDs so a K=3 split is
// [10, 30], [40, 60], [70, +inf); IDs between the ranges stay free.
func openRangeSeed(t *testing.T) (*fixture, *orchEnv) {
	t.Helper()
	fx := newFixture(t)
	fx.addEligible(10, 20, 30, 40, 50, 60, 70, 80, 90)
	e := newOrchEnv(t, fx.DB, 3)
	want := []struct {
		lower int64
		upper *int64
	}{{10, i64(30)}, {40, i64(60)}, {70, nil}}
	for i, w := range want {
		r := e.ranges[i]
		if r.Lower != w.lower || !reflect.DeepEqual(r.Upper, w.upper) {
			t.Fatalf("range %d = [%d, %v], want [%d, %v]", i, r.Lower, derefUpper(r.Upper), w.lower, derefUpper(w.upper))
		}
	}
	return fx, e
}

// assertLastRangeOpen checks RS R3 c1: the stored last range has no upper
// bound, and every other range has one.
func assertLastRangeOpen(t *testing.T, e *orchEnv) {
	t.Helper()
	st := stateMap(t, e.db)
	last := len(e.ranges) - 1
	if v, ok := st[rangeUpperKey(last)]; ok {
		t.Errorf("stored last range has %s = %d, want no upper bound", rangeUpperKey(last), v)
	}
	for i := 0; i < last; i++ {
		if _, ok := st[rangeUpperKey(i)]; !ok {
			t.Errorf("stored range %d has no upper bound", i)
		}
	}
	_, ranges, ok, err := e.migSt.loadSplit(context.Background())
	if err != nil || !ok {
		t.Fatalf("loadSplit: ok=%v err=%v", ok, err)
	}
	if ranges[last].Upper != nil {
		t.Errorf("loaded last range upper = %d, want nil", *ranges[last].Upper)
	}
}

// assertNonLastWithinUpper checks RS R3 c4: every non-last range's
// watermark (the highest ID it committed) is at most its stored upper bound.
func assertNonLastWithinUpper(t *testing.T, e *orchEnv) {
	t.Helper()
	st := stateMap(t, e.db)
	for i := 0; i < len(e.ranges)-1; i++ {
		wm, upper := st[rangeWatermarkKey(i)], st[rangeUpperKey(i)]
		if wm > upper {
			t.Errorf("range %d watermark %d is above its upper bound %d", i, wm, upper)
		}
	}
}

// TestOpenRangePicksUpBlockInsertedDuringRun: a block above the split-time
// maximum, inserted while the run is in progress (before the last range's
// next lookup), is migrated by the last range in the same run. Blocks inserted
// between two ranges, above a non-last range's upper bound, are never picked
// up by that range (RS R3 c1, c2, c4).
func TestOpenRangePicksUpBlockInsertedDuringRun(t *testing.T) {
	fx, e := openRangeSeed(t)
	const newID, gap0, gap1 = 100, 35, 65
	last := len(e.ranges) - 1

	// Hooks run on worker goroutines; the fixture is not safe to use there
	// (t.Fatalf, unguarded map), so inserts are handed to the test goroutine.
	type insertReq struct {
		ids  []int64
		done chan struct{}
	}
	reqs := make(chan insertReq)
	var once0 sync.Once
	lastCalls := 0 // only touched by the last range's worker
	ask := func(ids ...int64) {
		r := insertReq{ids: ids, done: make(chan struct{})}
		reqs <- r
		<-r.done
	}
	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		switch rangeIdx {
		case 0:
			once0.Do(func() { ask(gap0, gap1) })
		case last:
			// second batch start: after the last range's first batch has
			// committed, before its next lookup
			if lastCalls++; lastCalls == 2 {
				ask(newID)
			}
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- runRanges(ctx, e.migSt, e.repo, e.ranges, 2, nil) }()

	var err error
loop:
	for {
		select {
		case r := <-reqs:
			fx.addEligible(r.ids...)
			close(r.done)
		case err = <-errc:
			break loop
		}
	}
	if err != nil {
		t.Fatalf("runRanges: %v", err)
	}

	counts := migratedCountByDataID(t, fx.DB)
	if counts[newID] != 1 {
		t.Errorf("block %d inserted during the run migrated %d times, want 1", newID, counts[newID])
	}
	for _, id := range []int64{gap0, gap1} {
		if counts[id] != 0 {
			t.Errorf("block %d (between ranges, above a non-last upper bound) migrated %d times, want 0", id, counts[id])
		}
	}
	for _, id := range []int64{10, 20, 30, 40, 50, 60, 70, 80, 90} {
		if counts[id] != 1 {
			t.Errorf("block %d migrated %d times, want 1", id, counts[id])
		}
	}

	st := stateMap(t, fx.DB)
	if got := st[rangeWatermarkKey(last)]; got != newID {
		t.Errorf("last range watermark = %d, want %d", got, newID)
	}
	if got := st[rangeWatermarkKey(0)]; got != 30 {
		t.Errorf("range 0 watermark = %d, want its upper 30", got)
	}
	if got := st[rangeWatermarkKey(1)]; got != 60 {
		t.Errorf("range 1 watermark = %d, want its upper 60", got)
	}
	assertLastRangeOpen(t, e)
	assertNonLastWithinUpper(t, e)
}

// TestOpenRangeBlockAfterLastWorkerFinished: a block inserted after the last
// range's worker returned (while another range is still running) is not
// migrated in that run; the next run with the stored split migrates it via
// the last range. The same holds for a block inserted after a completed run
// (RS R3 c1–c4).
func TestOpenRangeBlockAfterLastWorkerFinished(t *testing.T) {
	fx, e := openRangeSeed(t)
	const lateID, afterRunID = 100, 200
	last := len(e.ranges) - 1
	splitBefore := splitOnly(stateMap(t, fx.DB))

	// Range 0 is held at its first batch start until the last range's worker
	// has returned and the late block is inserted.
	release := make(chan struct{})
	var once sync.Once
	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		if rangeIdx == 0 {
			once.Do(func() {
				select {
				case <-release:
				case <-time.After(10 * time.Second):
				}
			})
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs := make([]chan error, len(e.ranges))
	for i, r := range e.ranges {
		errs[i] = make(chan error, 1)
		go func() { errs[i] <- runRange(ctx, e.migSt, e.repo, r, 2, nil) }()
	}

	if err := <-errs[last]; err != nil {
		t.Fatalf("last range worker: %v", err)
	}
	// the last worker is done; range 0 has not started yet
	fx.addEligible(lateID)
	close(release)
	for i := 0; i < last; i++ {
		if err := <-errs[i]; err != nil {
			t.Fatalf("range %d worker: %v", i, err)
		}
	}

	counts := migratedCountByDataID(t, fx.DB)
	if counts[lateID] != 0 {
		t.Errorf("block %d inserted after the last worker finished migrated %d times in that run, want 0", lateID, counts[lateID])
	}
	st := stateMap(t, fx.DB)
	if got := st[rangeWatermarkKey(last)]; got != 90 {
		t.Errorf("last range watermark = %d, want 90", got)
	}
	assertNonLastWithinUpper(t, e)

	// Next run (full command) with the stored split picks the late block up.
	rerun := func(wantID int64) {
		t.Helper()
		if out, err := fx.run(context.Background(), "-b", "2", "-w", "3"); err != nil {
			t.Fatalf("rerun: %v\n%s", err, out)
		}
		st := stateMap(t, fx.DB)
		if got := splitOnly(st); !reflect.DeepEqual(got, splitBefore) {
			t.Errorf("stored split changed:\n before %v\n after  %v", splitBefore, got)
		}
		if got := st[rangeWatermarkKey(last)]; got != wantID {
			t.Errorf("last range watermark = %d, want %d", got, wantID)
		}
		if got := migratedCountByDataID(t, fx.DB)[wantID]; got != 1 {
			t.Errorf("block %d migrated %d times, want 1", wantID, got)
		}
		assertLastRangeOpen(t, e)
		assertNonLastWithinUpper(t, e)
	}
	rerun(lateID)

	// After a completed run: insert another block above the split-time max
	// and rerun.
	fx.addEligible(afterRunID)
	rerun(afterRunID)

	counts = migratedCountByDataID(t, fx.DB)
	for _, id := range fx.okIDs() {
		if counts[id] != 1 {
			t.Errorf("block %d migrated %d times, want 1", id, counts[id])
		}
	}
}
