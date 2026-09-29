package global_blocks

// Resume-after-interruption tests (T-027): a K=4 run is stopped by a fatal
// error mid-run, then rerun without the fault. Criteria RS R5 c5, c6 and
// PE R4 c5.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// resumeBarrierTimeout bounds every wait of the interruption choreography.
const resumeBarrierTimeout = 10 * time.Second

// rangeIndexOf returns the index of the stored range containing id, or -1.
func rangeIndexOf(ranges []storedRange, id int64) int {
	for _, r := range ranges {
		if r.contains(id) {
			return r.Index
		}
	}
	return -1
}

// interruptMidRun runs the K=4, -b 3 migration on the seedCrashFixture
// fixture and stops it deterministically:
//   - every range except faultRange commits exactly 2 batches, then blocks at
//     the start of its 3rd batch;
//   - once they all wait there, faultRange's 2nd batch fails in its
//     testHookBeforeBatchCommit with a fatal error (1 batch committed);
//   - the waiting ranges then stop without starting their 3rd batch.
//
// The waiting ranges stop by returning errStopped: returning nil would let
// them race the group cancellation into a 3rd batch. Their error can reach
// the group before faultRange's, so the command returns either one.
func interruptMidRun(t *testing.T, fx *fixture, faultRange int) (injected, errStopped, runErr error) {
	t.Helper()
	injected = errors.New("injected fatal error")
	errStopped = errors.New("stopped after the injected fault")

	var (
		mu      sync.Mutex
		starts  = map[int]int{}
		commits = map[int]int{}
	)
	waiting := make(chan int, crashK)
	faulted := make(chan struct{})

	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		mu.Lock()
		starts[rangeIdx]++
		n := starts[rangeIdx]
		mu.Unlock()
		if rangeIdx == faultRange || n != 3 {
			return nil
		}
		waiting <- rangeIdx
		select {
		case <-faulted:
			return errStopped
		case <-time.After(resumeBarrierTimeout):
			return fmt.Errorf("range %d: fault never happened", rangeIdx)
		}
	})
	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
		mu.Lock()
		commits[rangeIdx]++
		n := commits[rangeIdx]
		mu.Unlock()
		if rangeIdx != faultRange || n != 2 {
			return nil
		}
		timeout := time.After(resumeBarrierTimeout)
		for i := 0; i < crashK-1; i++ {
			select {
			case <-waiting:
			case <-timeout:
				return fmt.Errorf("only %d ranges reached their 3rd batch", i)
			}
		}
		close(faulted)
		return injected
	})

	_, runErr = fx.run(context.Background(), crashRunArgs()...)

	// later runs must not see these hooks
	testHookBatchStart, testHookBeforeBatchCommit = nil, nil
	return injected, errStopped, runErr
}

// TestResumeAfterInterruption: after a fatal error mid-run, each range's
// remaining work is exactly the eligible IDs in (watermark, upper]; a rerun
// continues every range from its own watermark, migrates nothing twice,
// keeps the rows committed before the interruption untouched, and completes.
func TestResumeAfterInterruption(t *testing.T) {
	fx := newFixture(t)
	seedCrashFixture(fx)
	ctx := context.Background()
	eligible := fx.eligibleIDs()
	const faultRange = 2

	injected, errStopped, err := interruptMidRun(t, fx, faultRange)
	if !errors.Is(err, injected) && !errors.Is(err, errStopped) {
		t.Fatalf("interrupted run error = %v, want the injected fault", err)
	}

	// --- state after the interruption
	ranges := assertWatermarkInvariants(t, fx)
	if len(ranges) != crashK {
		t.Fatalf("stored split has %d ranges, want %d", len(ranges), crashK)
	}
	if ranges[crashK-1].Upper != nil {
		t.Fatalf("last range upper = %d, want open-ended", *ranges[crashK-1].Upper)
	}
	for _, r := range ranges {
		batches := 2
		if r.Index == faultRange {
			batches = 1
		}
		inRange := idsInRange(eligible, r.idRange)
		if want := inRange[batches*crashBatch-1]; r.Watermark != want {
			t.Fatalf("range %d watermark = %d, want %d (%d batches committed)", r.Index, r.Watermark, want, batches)
		}
	}

	// --- RS R5 c5: remaining work per range is exactly (watermark, upper]
	repo, err := newPrepareRepository(ctx, fx.DB)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	remaining := map[int][]int64{}
	for _, r := range ranges {
		var want []int64
		for _, id := range eligible {
			if id > r.Watermark && (r.Upper == nil || id <= *r.Upper) {
				want = append(want, id)
			}
		}
		remaining[r.Index] = want

		n, err := repo.getRemainingCountInRange(ctx, r.Watermark, r.Upper)
		if err != nil {
			t.Fatalf("getRemainingCountInRange(range %d): %v", r.Index, err)
		}
		if n != int64(len(want)) {
			t.Errorf("range %d remaining count = %d, want %d", r.Index, n, len(want))
		}
		ids, err := repo.getGlobalBlocksIdsInRange(ctx, r.Watermark, r.Upper, 1000)
		if err != nil {
			t.Fatalf("getGlobalBlocksIdsInRange(range %d): %v", r.Index, err)
		}
		if d := diffIDs(ids, want); d != "" {
			t.Errorf("range %d remaining IDs: %s", r.Index, d)
		}
	}

	pre := takeSnapshot(t, fx.DB)
	preMigrated := migratedDataIDs(t, fx.DB)
	preFailed := failedIDs(t, fx.DB)
	if len(preMigrated) == 0 {
		t.Fatal("nothing committed before the interruption")
	}

	// --- rerun without the fault, recording what each range processes
	var mu sync.Mutex
	processed := map[int][]int64{}
	commits := map[int]int{}
	setTestHook(t, &testHookBeforeBlock, func(dataID int64, done int) error {
		mu.Lock()
		defer mu.Unlock()
		r := rangeIndexOf(ranges, dataID)
		processed[r] = append(processed[r], dataID)
		return nil
	})
	setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
		mu.Lock()
		defer mu.Unlock()
		commits[rangeIdx]++
		return nil
	})
	out, err := fx.run(ctx, crashRunArgs()...)
	if err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
	if strings.Contains(out, "Notice") {
		t.Errorf("rerun with the stored K printed a notice:\n%s", out)
	}

	// PE R4 c5: every range continued from its own watermark and processed
	// exactly its remaining IDs, in ascending order.
	for _, r := range ranges {
		got := processed[r.Index]
		want := remaining[r.Index]
		if !slices.Equal(got, want) {
			t.Errorf("range %d processed %v on rerun, want %v", r.Index, got, want)
		}
		if len(got) > 0 && got[0] <= r.Watermark {
			t.Errorf("range %d restarted at %d, at or below its watermark %d", r.Index, got[0], r.Watermark)
		}
		if wantCommits := (len(want) + crashBatch - 1) / crashBatch; commits[r.Index] != wantCommits {
			t.Errorf("range %d committed %d batches on rerun, want %d", r.Index, commits[r.Index], wantCommits)
		}
	}
	if extra := processed[-1]; len(extra) != 0 {
		t.Errorf("rerun processed IDs outside every range: %v", extra)
	}

	// RS R5 c6: nothing at or below a pre-rerun watermark was migrated again,
	// and the rows committed before the interruption kept their insert IDs.
	for id, n := range migratedCountByDataID(t, fx.DB) {
		if n != 1 {
			t.Errorf("block %d migrated %d times", id, n)
		}
	}
	post := takeSnapshot(t, fx.DB)
	for _, table := range migratedTables {
		before, after := pre.Migrated[table], post.Migrated[table]
		if len(after) < len(before) || !reflect.DeepEqual(after[:len(before)], before) {
			t.Errorf("%s: rows committed before the interruption changed (before %d rows, after %d)", table, len(before), len(after))
		}
	}
	postFailed := map[int64]failedRow{}
	for _, r := range post.Failed {
		postFailed[r.DataID] = r
	}
	for _, r := range pre.Failed {
		if postFailed[r.DataID] != r {
			t.Errorf("failed row %+v changed to %+v", r, postFailed[r.DataID])
		}
	}
	splitKeys := func(rows []stateRow) []stateRow {
		var out []stateRow
		for _, r := range rows {
			if !strings.HasSuffix(r.Key, "_watermark") {
				out = append(out, r)
			}
		}
		return out
	}
	if !reflect.DeepEqual(splitKeys(pre.State), splitKeys(post.State)) {
		t.Errorf("split keys changed on rerun:\n before %v\n after  %v", pre.State, post.State)
	}
	for _, id := range preMigrated {
		if slices.Contains(processed[rangeIndexOf(ranges, id)], id) {
			t.Errorf("block %d (migrated before the interruption) was processed again", id)
		}
	}
	for _, id := range preFailed {
		if slices.Contains(processed[rangeIndexOf(ranges, id)], id) {
			t.Errorf("block %d (failed before the interruption) was processed again", id)
		}
	}

	// --- final state is complete
	assertWatermarkInvariants(t, fx)
	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("final migrated: %s", d)
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Errorf("final failed: %s", d)
	}
	final, ok := loadStoredSplit(t, fx)
	if !ok {
		t.Fatal("split missing after rerun")
	}
	for _, r := range final {
		inRange := idsInRange(eligible, r.idRange)
		if max := slices.Max(inRange); r.Watermark < max {
			t.Errorf("range %d final watermark %d < max eligible %d", r.Index, r.Watermark, max)
		}
	}
}
