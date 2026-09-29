package global_blocks

// T-016: integration tests for stored split reuse and the K-mismatch notice
// (RS R5 c1–c4).

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// splitNotice is the notice ensureSplit prints when requested K differs from
// the stored split's K.
func splitNotice(stored, requested int) string {
	return fmt.Sprintf("Notice: stored split has K=%d, requested K=%d; using stored split", stored, requested)
}

// splitBoundRows returns the split_k, range_*_lower and range_*_upper state
// rows (watermarks excluded, since workers move them).
func splitBoundRows(t *testing.T, fx *fixture) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for k, v := range stateMap(t, fx.DB) {
		if k == splitKKey {
			out[k] = v
			continue
		}
		if m := rangeKeyRe.FindStringSubmatch(k); m != nil && m[2] != "watermark" {
			out[k] = v
		}
	}
	return out
}

// assertNotice checks the mismatch notice appears exactly once when requested
// != stored and never otherwise.
func assertNotice(t *testing.T, out string, stored, requested int) {
	t.Helper()
	n := strings.Count(out, "Notice: stored split has")
	if requested == stored {
		if n != 0 {
			t.Fatalf("requested K=%d equals stored K: unexpected notice in output:\n%s", requested, out)
		}
		return
	}
	if n != 1 || !strings.Contains(out, splitNotice(stored, requested)) {
		t.Fatalf("want notice %q exactly once (found %d):\n%s", splitNotice(stored, requested), n, out)
	}
}

func TestEnsureSplitReusesStoredSplit(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.addEligibleN(5)
	fx.addWrongNode()
	fx.addEligibleN(6)
	fx.addFailing(failInvalidIRI)
	fx.addLegacyAPI()
	fx.addEligibleN(7)
	st, repo := newSplitEnv(t, fx.DB)

	const s = 4
	created, out, err := runEnsureSplit(t, ctx, fx.DB, st, repo, s)
	if err != nil {
		t.Fatalf("create split: %v", err)
	}
	if len(created) != s {
		t.Fatalf("created %d ranges, want %d", len(created), s)
	}
	assertNotice(t, out, s, s)
	before := stateRows(t, fx.DB)

	// new eligible blocks after the split: a recompute would move bounds
	fx.addEligibleN(9)
	fx.addEligible(fx.maxSeededID() + 1000)

	for _, requested := range []int{4, 2, 8, 1, 4} {
		t.Run(fmt.Sprintf("requested=%d", requested), func(t *testing.T) {
			got, out, err := runEnsureSplit(t, ctx, fx.DB, st, repo, requested)
			if err != nil {
				t.Fatalf("ensureSplit: %v", err)
			}
			if !reflect.DeepEqual(got, created) {
				t.Fatalf("ensureSplit returned %+v, want stored split %+v", got, created)
			}
			if after := stateRows(t, fx.DB); !reflect.DeepEqual(after, before) {
				t.Fatalf("state rows changed:\n got %v\nwant %v", after, before)
			}
			assertNotice(t, out, s, requested)
			assertNoMigratedData(t, fx.DB)
		})
	}
}

func TestEnsureSplitReusesStoreSplitRows(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.addEligibleN(3)
	fx.addNoAPIVersion()
	fx.addEligibleN(10)
	st, repo := newSplitEnv(t, fx.DB)

	// a split written directly by storeSplit, deliberately not what
	// computeSplit would produce for the current eligible IDs
	ids := fx.eligibleIDs()
	split := []idRange{
		{Index: 0, Lower: ids[0], Upper: i64(ids[0])},
		{Index: 1, Lower: ids[1], Upper: i64(ids[2])},
		{Index: 2, Lower: ids[3], Upper: i64(ids[9])},
		{Index: 3, Lower: ids[10]},
	}
	if err := st.storeSplit(ctx, fx.DB, split); err != nil {
		t.Fatalf("storeSplit: %v", err)
	}
	before := stateRows(t, fx.DB)

	for _, requested := range []int{4, 2, 8} {
		got, out, err := runEnsureSplit(t, ctx, fx.DB, st, repo, requested)
		if err != nil {
			t.Fatalf("requested=%d: ensureSplit: %v", requested, err)
		}
		if len(got) != len(split) {
			t.Fatalf("requested=%d: %d ranges, want %d", requested, len(got), len(split))
		}
		for i := range got {
			if !reflect.DeepEqual(got[i].idRange, split[i]) || got[i].Watermark != split[i].Lower-1 {
				t.Fatalf("requested=%d: range %d = %+v, want %+v", requested, i, got[i], split[i])
			}
		}
		if after := stateRows(t, fx.DB); !reflect.DeepEqual(after, before) {
			t.Fatalf("requested=%d: state rows changed:\n got %v\nwant %v", requested, after, before)
		}
		assertNotice(t, out, 4, requested)
	}
}

// TestCommandReusesStoredSplit runs the command end to end against a stored
// K=4 split with -w 2 and then -w 4: bounds and split_k never change (the
// watermarks do, as workers progress) and the notice is printed only for -w 2.
func TestCommandReusesStoredSplit(t *testing.T) {
	ctx := context.Background()

	t.Run("w=2", func(t *testing.T) {
		fx := newFixture(t)
		fx.addEligibleN(9)
		fx.addWrongNode()
		fx.addEligibleN(9)
		st, repo := newSplitEnv(t, fx.DB)
		if _, _, err := runEnsureSplit(t, ctx, fx.DB, st, repo, 4); err != nil {
			t.Fatalf("create split: %v", err)
		}
		bounds := splitBoundRows(t, fx)
		if bounds[splitKKey] != 4 {
			t.Fatalf("split_k = %d, want 4", bounds[splitKKey])
		}

		out, err := fx.run(ctx, "-b", "3", "-w", "2")
		if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		assertNotice(t, out, 4, 2)
		if got := splitBoundRows(t, fx); !reflect.DeepEqual(got, bounds) {
			t.Fatalf("split bounds changed:\n got %v\nwant %v", got, bounds)
		}
		if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
			t.Fatalf("migrated IDs: %s", d)
		}
	})

	t.Run("w=4", func(t *testing.T) {
		fx := newFixture(t)
		fx.addEligibleN(9)
		fx.addLegacyAPI()
		fx.addEligibleN(9)
		st, repo := newSplitEnv(t, fx.DB)
		if _, _, err := runEnsureSplit(t, ctx, fx.DB, st, repo, 4); err != nil {
			t.Fatalf("create split: %v", err)
		}
		bounds := splitBoundRows(t, fx)

		out, err := fx.run(ctx, "-b", "3", "-w", "4")
		if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		assertNotice(t, out, 4, 4)
		if got := splitBoundRows(t, fx); !reflect.DeepEqual(got, bounds) {
			t.Fatalf("split bounds changed:\n got %v\nwant %v", got, bounds)
		}
	})
}
