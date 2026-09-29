package global_blocks

// End-to-end smoke tests of the range-split migration (T-013/T-014). The
// detailed per-criterion tests live in later tasks; these prove the pieces
// fit together. Golden equivalence (K=1 and --failed vs the pre-change tool)
// lives in golden_equivalence_integration_test.go (T-028).

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRangeWorkerSmoke(t *testing.T) {
	fx := newFixture(t)
	fx.addEligibleN(7)
	fx.addFailing(failMalformedRules)
	fx.addWrongNode()
	fx.addEligibleN(4)
	fx.addFailing(failInvalidIRI)
	fx.addLegacyAPI()
	fx.addEligibleN(5)
	fx.addFailing(failMissingCollectionType)
	fx.addEligibleN(2)

	ctx := context.Background()
	out, err := fx.run(ctx, "-b", "3", "-w", "3")
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	// every OK block migrated exactly once
	counts := migratedCountByDataID(t, fx.DB)
	for _, id := range fx.okIDs() {
		if counts[id] != 1 {
			t.Errorf("block %d migrated %d times, want 1", id, counts[id])
		}
	}
	var migrated []int64
	for id := range counts {
		migrated = append(migrated, id)
	}
	slices.Sort(migrated)
	if d := diffIDs(migrated, fx.okIDs()); d != "" {
		t.Fatalf("migrated IDs: %s", d)
	}

	// failing blocks land in the failed table
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Fatalf("failed IDs: %s", d)
	}

	// three ranges, each watermark at or past its last eligible ID
	st := stateMap(t, fx.DB)
	if st[splitKKey] != 3 {
		t.Fatalf("split_k = %d, want 3 (state %v)", st[splitKKey], st)
	}
	eligible := fx.eligibleIDs()
	for i := 0; i < 3; i++ {
		lower, upper, hasUpper := st[rangeLowerKey(i)], st[rangeUpperKey(i)], i < 2
		var last int64
		for _, id := range eligible {
			if id >= lower && (!hasUpper || id <= upper) {
				last = id
			}
		}
		if last == 0 {
			t.Fatalf("range %d has no eligible IDs (state %v)", i, st)
		}
		if wm := st[rangeWatermarkKey(i)]; wm < last {
			t.Errorf("range %d watermark = %d, want >= %d", i, wm, last)
		}
	}

	// rerun: nothing new is migrated, nothing changes
	snap := takeSnapshot(t, fx.DB)
	out, err = fx.run(ctx, "-b", "3", "-w", "3")
	if err != nil {
		t.Fatalf("rerun failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Nothing to migrate") {
		t.Errorf("rerun output lacks 'Nothing to migrate':\n%s", out)
	}
	if !reflect.DeepEqual(snap, takeSnapshot(t, fx.DB)) {
		t.Fatal("rerun changed the database")
	}
}
