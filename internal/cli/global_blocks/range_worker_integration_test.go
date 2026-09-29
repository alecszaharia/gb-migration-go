package global_blocks

// Integration tests of a single range worker (T-019): runRange called
// directly on one range of a stored split. Criteria RS R4 c1–c3, c6 and
// PE R3 c2–c8.

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const rwBatch = 3

// rwEnv is a seeded fixture with a stored K=3 split and the migration state
// and repository a worker needs.
//
// Seeded IDs (eligible unless noted):
//
//	range 0: 1..6                      upper 6
//	range 1: 11, 12 (failing), 13, 15, 16, 17 and 14 (wrong node, ineligible)  upper 17
//	range 2: 21..26                    open-ended
type rwEnv struct {
	fx      *fixture
	migSt   *state
	repo    *repository
	ranges  []storedRange
	failing int64
	kind    failKind
}

func newRWEnv(t *testing.T) *rwEnv {
	t.Helper()
	fx := newFixture(t)
	fx.addEligible(1, 2, 3, 4, 5, 6)
	fx.addEligible(11)
	const kind = failMalformedRules
	failing := fx.addFailing(kind, 12)[0]
	fx.addEligible(13)
	fx.addWrongNode(14)
	fx.addEligible(15, 16, 17)
	fx.addEligible(21, 22, 23, 24, 25, 26)

	ctx := context.Background()
	repo, err := newPrepareRepository(ctx, fx.DB)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	migSt := &state{}
	if err := migSt.init(ctx, fx.DB); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	ranges, err := ensureSplit(ctx, fx.DB, migSt, repo, 3)
	if err != nil {
		t.Fatalf("ensureSplit: %v", err)
	}

	// sanity: the split is what the tests below assume, and it is persisted
	wantBounds := []struct {
		lower int64
		upper *int64
	}{{1, i64(6)}, {11, i64(17)}, {21, nil}}
	if len(ranges) != len(wantBounds) {
		t.Fatalf("split has %d ranges, want %d: %+v", len(ranges), len(wantBounds), ranges)
	}
	for i, w := range wantBounds {
		r := ranges[i]
		if r.Index != i || r.Lower != w.lower || !reflect.DeepEqual(r.Upper, w.upper) || r.Watermark != w.lower-1 {
			t.Fatalf("range %d = %+v (upper %v), want lower %d upper %v watermark %d", i, r, derefUpper(r.Upper), w.lower, derefUpper(w.upper), w.lower-1)
		}
	}
	_, stored, ok, err := migSt.loadSplit(ctx)
	if err != nil || !ok {
		t.Fatalf("loadSplit: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(stored, ranges) {
		t.Fatalf("stored split %+v != ensureSplit result %+v", stored, ranges)
	}

	return &rwEnv{fx: fx, migSt: migSt, repo: repo, ranges: ranges, failing: failing, kind: kind}
}

func derefUpper(u *int64) any {
	if u == nil {
		return "nil"
	}
	return *u
}

// rwObservation is the committed database state seen at one point in time.
type rwObservation struct {
	migrated []int64
	failed   []int64
	state    map[string]int64
}

func (e *rwEnv) observe(t *testing.T) rwObservation {
	t.Helper()
	return rwObservation{
		migrated: migratedDataIDs(t, e.fx.DB),
		failed:   failedIDs(t, e.fx.DB),
		state:    stateMap(t, e.fx.DB),
	}
}

// rwRecorder records the committed state at every testHookBatchStart (for
// rangeIdx) and counts testHookBeforeBatchCommit calls.
type rwRecorder struct {
	starts       []rwObservation
	startIdx     []int
	commitCalls  int
	commitHookFn func(call int) error // optional, call is 1-based
}

func (e *rwEnv) record(t *testing.T) *rwRecorder {
	t.Helper()
	rec := &rwRecorder{}
	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		rec.startIdx = append(rec.startIdx, rangeIdx)
		rec.starts = append(rec.starts, e.observe(t))
		return nil
	})
	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
		rec.commitCalls++
		if rec.commitHookFn != nil {
			return rec.commitHookFn(rec.commitCalls)
		}
		return nil
	})
	return rec
}

// newIDs returns the sorted IDs in after that are not in before.
func newIDs(before, after []int64) []int64 {
	var out []int64
	for _, id := range after {
		if !slices.Contains(before, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// batchesFrom derives each committed batch's IDs (newly migrated ∪ newly
// failed) from consecutive observations.
func batchesFrom(obs []rwObservation) [][]int64 {
	var batches [][]int64
	for i := 1; i < len(obs); i++ {
		ids := append(newIDs(obs[i-1].migrated, obs[i].migrated), newIDs(obs[i-1].failed, obs[i].failed)...)
		slices.Sort(ids)
		batches = append(batches, ids)
	}
	return batches
}

// withoutKey returns a copy of m without key.
func withoutKey(m map[string]int64, key string) map[string]int64 {
	c := maps.Clone(m)
	delete(c, key)
	return c
}

// TestRangeWorkerBatches runs range 1 to completion and checks batch
// contents, per-batch watermarks and the failing block (RS R4 c1–c3, c6;
// PE R3 c2–c4, c6).
func TestRangeWorkerBatches(t *testing.T) {
	e := newRWEnv(t)
	ctx := context.Background()
	r := e.ranges[1]
	wmKey := rangeWatermarkKey(r.Index)
	rec := e.record(t)

	if err := runRange(ctx, e.migSt, e.repo, r, rwBatch, nil); err != nil {
		t.Fatalf("runRange: %v", err)
	}
	final := e.observe(t)

	// batch starts: 2 batches with data + 1 empty lookup
	if len(rec.starts) != 3 {
		t.Fatalf("testHookBatchStart called %d times, want 3", len(rec.starts))
	}
	for _, idx := range rec.startIdx {
		if idx != r.Index {
			t.Errorf("hook called with range %d, want %d", idx, r.Index)
		}
	}
	if rec.commitCalls != 2 {
		t.Errorf("testHookBeforeBatchCommit called %d times, want 2", rec.commitCalls)
	}
	// the last start was the empty lookup: nothing changed after it
	if last := rec.starts[len(rec.starts)-1]; !reflect.DeepEqual(last, final) {
		t.Errorf("state changed after the empty lookup:\n before %+v\n after  %+v", last, final)
	}

	obs := append(slices.Clone(rec.starts[:len(rec.starts)-1]), final)
	batches := batchesFrom(obs)
	wantBatches := [][]int64{{11, 12, 13}, {15, 16, 17}}
	if !reflect.DeepEqual(batches, wantBatches) {
		t.Fatalf("batches = %v, want %v", batches, wantBatches)
	}

	var all []int64
	for i, b := range batches {
		// PE R3 c4: batch size bounded
		if len(b) == 0 || len(b) > rwBatch {
			t.Errorf("batch %d size %d, want 1..%d", i, len(b), rwBatch)
		}
		// PE R3 c2, c3: only this range's IDs
		for _, id := range b {
			if !r.contains(id) {
				t.Errorf("batch %d id %d outside range [%d, %v]", i, id, r.Lower, derefUpper(r.Upper))
			}
		}
		all = append(all, b...)

		// RS R4 c1–c3: after the commit, watermark == batch max, other
		// ranges' watermarks and split keys untouched.
		before, after := obs[i].state, obs[i+1].state
		if got, want := after[wmKey], slices.Max(b); got != want {
			t.Errorf("after batch %d: %s = %d, want %d", i, wmKey, got, want)
		}
		if !reflect.DeepEqual(withoutKey(before, wmKey), withoutKey(after, wmKey)) {
			t.Errorf("after batch %d: other state keys changed:\n before %v\n after  %v", i, before, after)
		}
	}
	// PE R3 c2: ascending across batches, no repeats
	if !slices.IsSorted(all) || len(slices.Compact(slices.Clone(all))) != len(all) {
		t.Errorf("processed IDs not strictly ascending: %v", all)
	}
	// the ineligible block inside the range was never touched
	if slices.Contains(all, 14) {
		t.Errorf("ineligible block 14 was processed")
	}

	// RS R4 c6 / PE R3 c6: failing block recorded, peers committed,
	// watermark passes it.
	if slices.Contains(final.migrated, e.failing) {
		t.Errorf("failing block %d has migrated rows", e.failing)
	}
	counts := migratedCountByDataID(t, e.fx.DB)
	for _, id := range []int64{11, 13, 15, 16, 17} {
		if counts[id] != 1 {
			t.Errorf("block %d migrated %d times, want 1", id, counts[id])
		}
	}
	if !reflect.DeepEqual(final.migrated, []int64{11, 13, 15, 16, 17}) {
		t.Errorf("migrated = %v, want only range 1's ok blocks", final.migrated)
	}
	rows := failedRows(t, e.fx.DB)
	if len(rows) != 1 || rows[0].DataID != e.failing {
		t.Fatalf("failed rows = %+v, want one row for %d", rows, e.failing)
	}
	if !strings.Contains(rows[0].Error, e.kind.errorSubstring()) {
		t.Errorf("failed error %q lacks %q", rows[0].Error, e.kind.errorSubstring())
	}
	if final.state[wmKey] <= e.failing {
		t.Errorf("%s = %d, want > failing block %d", wmKey, final.state[wmKey], e.failing)
	}
	if got := final.state[wmKey]; got != 17 {
		t.Errorf("final %s = %d, want 17", wmKey, got)
	}
}

// TestRangeWorkerBatchOwnTx injects an error before batch 2's commit: batch 1
// stays committed, batch 2 leaves no trace (PE R3 c5).
func TestRangeWorkerBatchOwnTx(t *testing.T) {
	e := newRWEnv(t)
	ctx := context.Background()
	r := e.ranges[1]
	wmKey := rangeWatermarkKey(r.Index)
	initial := e.observe(t)

	injected := errors.New("injected pre-commit failure")
	rec := e.record(t)
	rec.commitHookFn = func(call int) error {
		if call == 2 {
			return injected
		}
		return nil
	}

	err := runRange(ctx, e.migSt, e.repo, r, rwBatch, nil)
	if !errors.Is(err, injected) {
		t.Fatalf("runRange err = %v, want %v", err, injected)
	}
	if rec.commitCalls != 2 {
		t.Errorf("testHookBeforeBatchCommit called %d times, want 2", rec.commitCalls)
	}

	final := e.observe(t)
	if !reflect.DeepEqual(final.migrated, []int64{11, 13}) {
		t.Errorf("migrated = %v, want batch 1 ok blocks [11 13]", final.migrated)
	}
	if !reflect.DeepEqual(final.failed, []int64{e.failing}) {
		t.Errorf("failed = %v, want [%d]", final.failed, e.failing)
	}
	if got := final.state[wmKey]; got != 13 {
		t.Errorf("%s = %d, want batch 1 max 13", wmKey, got)
	}
	if !reflect.DeepEqual(withoutKey(initial.state, wmKey), withoutKey(final.state, wmKey)) {
		t.Errorf("other state keys changed:\n before %v\n after  %v", initial.state, final.state)
	}
}

// TestRangeWorkerNoRemainingWork: a range with nothing left migrates nothing
// and returns nil without any batch commit (PE R3 c7).
func TestRangeWorkerNoRemainingWork(t *testing.T) {
	ctx := context.Background()

	t.Run("watermark at upper", func(t *testing.T) {
		e := newRWEnv(t)
		r := e.ranges[0]
		r.Watermark = *r.Upper
		before := takeSnapshot(t, e.fx.DB)
		rec := e.record(t)

		if err := runRange(ctx, e.migSt, e.repo, r, rwBatch, nil); err != nil {
			t.Fatalf("runRange: %v", err)
		}
		if len(rec.starts) != 1 || rec.commitCalls != 0 {
			t.Errorf("batch starts %d, commits %d; want 1, 0", len(rec.starts), rec.commitCalls)
		}
		if !reflect.DeepEqual(before, takeSnapshot(t, e.fx.DB)) {
			t.Error("database changed")
		}
	})

	t.Run("all blocks migrated", func(t *testing.T) {
		e := newRWEnv(t)
		if err := runRange(ctx, e.migSt, e.repo, e.ranges[1], rwBatch, nil); err != nil {
			t.Fatalf("first runRange: %v", err)
		}
		_, ranges, ok, err := e.migSt.loadSplit(ctx)
		if err != nil || !ok {
			t.Fatalf("loadSplit: ok=%v err=%v", ok, err)
		}
		r := ranges[1]
		if r.Watermark != 17 {
			t.Fatalf("reloaded range 1 watermark = %d, want 17", r.Watermark)
		}
		before := takeSnapshot(t, e.fx.DB)
		rec := e.record(t)

		if err := runRange(ctx, e.migSt, e.repo, r, rwBatch, nil); err != nil {
			t.Fatalf("second runRange: %v", err)
		}
		if len(rec.starts) != 1 || rec.commitCalls != 0 {
			t.Errorf("batch starts %d, commits %d; want 1, 0", len(rec.starts), rec.commitCalls)
		}
		if !reflect.DeepEqual(before, takeSnapshot(t, e.fx.DB)) {
			t.Error("database changed")
		}
	})
}

// TestRangeWorkerReturnsAfterEmptyLookup: the open-ended last range returns
// as soon as a lookup comes back empty, without polling (PE R3 c8).
func TestRangeWorkerReturnsAfterEmptyLookup(t *testing.T) {
	e := newRWEnv(t)
	r := e.ranges[2]
	if r.Upper != nil {
		t.Fatalf("range 2 upper = %v, want open-ended", *r.Upper)
	}
	rec := e.record(t)

	const bound = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()

	start := time.Now()
	err := runRange(ctx, e.migSt, e.repo, r, rwBatch, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runRange: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("context expired (%v) before runRange returned", ctx.Err())
	}
	if elapsed > bound/2 {
		t.Errorf("runRange took %v, want well under %v", elapsed, bound)
	}

	// 6 blocks / batch 3 = 2 batches, plus the single empty lookup
	if len(rec.starts) != 3 {
		t.Errorf("testHookBatchStart called %d times, want 3 (2 batches + 1)", len(rec.starts))
	}
	if rec.commitCalls != 2 {
		t.Errorf("testHookBeforeBatchCommit called %d times, want 2", rec.commitCalls)
	}
	if got := migratedDataIDs(t, e.fx.DB); !reflect.DeepEqual(got, []int64{21, 22, 23, 24, 25, 26}) {
		t.Errorf("migrated = %v, want [21..26]", got)
	}
	if got := stateMap(t, e.fx.DB)[rangeWatermarkKey(2)]; got != 26 {
		t.Errorf("range 2 watermark = %d, want 26", got)
	}
}
