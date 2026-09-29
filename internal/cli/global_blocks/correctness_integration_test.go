package global_blocks

// Exactly-once correctness across worker counts (T-025, PE R6 c1-c4).
//
// Every run starts from a fresh fixture built by the same seed function
// (seedGoldenFixture: 51 OK blocks, 9 failing blocks, 9 ineligible rows,
// sparse IDs) and runs the tool to completion with -w K and a small batch, so
// every range spans several batches with failing blocks mixed in.

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// correctnessBatch is small so each of the K ranges needs several batches.
const correctnessBatch = "3"

// correctnessOutcome is what one K run left in the database.
type correctnessOutcome struct {
	migratedIDs []int64
	failedIDs   []int64
	golden      goldenSnapshot // normalised migrated data + failed table
}

// runCorrectnessK seeds a fresh golden fixture, runs the tool to completion
// with k workers and asserts the exactly-once properties for that run.
func runCorrectnessK(t *testing.T, k int) correctnessOutcome {
	t.Helper()
	fx := newFixture(t)
	seedGoldenFixture(fx)

	out, err := fx.run(context.Background(), "-b", correctnessBatch, "-w", fmt.Sprint(k))
	if err != nil {
		t.Fatalf("run -w %d failed: %v\n%s", k, err, out)
	}

	// the run really used K ranges (60 eligible blocks >= every K tested)
	if st := stateMap(t, fx.DB); st[splitKKey] != int64(k) {
		t.Fatalf("split_k = %d, want %d", st[splitKKey], k)
	}

	counts := migratedCountByDataID(t, fx.DB)
	migrated := make([]int64, 0, len(counts))
	for id, n := range counts {
		// PE R6 c2: no block migrated more than once
		if n != 1 {
			t.Errorf("block %d has %d migrated records, want 1", id, n)
		}
		migrated = append(migrated, id)
	}
	slices.Sort(migrated)
	failed := failedIDs(t, fx.DB)

	eligible := fx.eligibleIDs()
	isEligible := make(map[int64]bool, len(eligible))
	for _, id := range eligible {
		isEligible[id] = true
	}
	isFailed := make(map[int64]bool, len(failed))
	for _, id := range failed {
		isFailed[id] = true
		if !isEligible[id] {
			t.Errorf("failed row for non-eligible block %d", id)
		}
	}
	for _, id := range migrated {
		if !isEligible[id] {
			t.Errorf("non-eligible block %d was migrated", id)
		}
	}
	// PE R6 c1/c3: every eligible ID has migrated data XOR a failed row
	for _, id := range eligible {
		_, hasData := counts[id]
		switch {
		case hasData && isFailed[id]:
			t.Errorf("block %d has both migrated data and a failed row", id)
		case !hasData && !isFailed[id]:
			t.Errorf("block %d has neither migrated data nor a failed row", id)
		}
	}

	// and the split is exactly the fixture's intended classification
	if d := diffIDs(migrated, fx.okIDs()); d != "" {
		t.Errorf("migrated IDs vs fixture OK IDs: %s", d)
	}
	if d := diffIDs(failed, fx.failingIDs()); d != "" {
		t.Errorf("failed IDs vs fixture failing IDs: %s", d)
	}

	return correctnessOutcome{migratedIDs: migrated, failedIDs: failed, golden: captureGolden(t, fx.DB)}
}

func TestCorrectnessExactlyOnceAcrossK(t *testing.T) {
	outcomes := map[int]correctnessOutcome{}
	for _, k := range []int{1, 2, 4, 8} {
		t.Run(fmt.Sprintf("K=%d", k), func(t *testing.T) {
			outcomes[k] = runCorrectnessK(t, k)
		})
	}
	if t.Failed() {
		return
	}

	// PE R6 c4: K=1 and K=4 produce the same migrated and failed sets
	one, four := outcomes[1], outcomes[4]
	if d := diffIDs(four.migratedIDs, one.migratedIDs); d != "" {
		t.Errorf("K=4 vs K=1 migrated IDs: %s", d)
	}
	if d := diffIDs(four.failedIDs, one.failedIDs); d != "" {
		t.Errorf("K=4 vs K=1 failed IDs: %s", d)
	}

	// stronger: the normalised migrated data and failed table are identical
	// for every K (content, rules and error messages, not just IDs)
	for _, k := range []int{2, 4, 8} {
		if !reflect.DeepEqual(outcomes[k].golden, one.golden) {
			t.Errorf("K=%d normalised outcome differs from K=1:\n%s", k, diffGolden(one.golden, outcomes[k].golden))
		}
	}
}
