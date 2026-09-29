package global_blocks

// Light tests of the concurrent orchestrator (T-017) and the --failed
// isolation (T-018). The detailed suites live in later tasks.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRangeError(t *testing.T) {
	base := errors.New("boom")
	upper := int64(179)
	tests := []struct {
		name string
		r    storedRange
		want string
	}{
		{"bounded", storedRange{idRange: idRange{Index: 2, Lower: 120, Upper: &upper}}, "range 2 [120, 179]: boom"},
		{"open-ended", storedRange{idRange: idRange{Index: 3, Lower: 180}}, "range 3 [180, +inf]: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rangeError(tt.r, base)
			if err.Error() != tt.want {
				t.Errorf("got %q, want %q", err.Error(), tt.want)
			}
			if !errors.Is(err, base) {
				t.Error("wrapped error lost")
			}
		})
	}
}

// seedFourRangeFixture seeds 24 eligible blocks (some failing) so that a
// K=4 split gives every range work.
func seedFourRangeFixture(fx *fixture) {
	fx.addEligibleN(5)
	fx.addFailing(failMalformedRules)
	fx.addEligibleN(5)
	fx.addFailing(failInvalidIRI)
	fx.addWrongNode()
	fx.addEligibleN(5)
	fx.addFailing(failMissingCollectionType)
	fx.addEligibleN(6)
}

func TestRunRangesConcurrentExactlyOnce(t *testing.T) {
	fx := newFixture(t)
	seedFourRangeFixture(fx)

	out, err := fx.run(context.Background(), "-b", "2", "-w", "4")
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	counts := migratedCountByDataID(t, fx.DB)
	var migrated []int64
	for id, n := range counts {
		if n != 1 {
			t.Errorf("block %d migrated %d times, want 1", id, n)
		}
		migrated = append(migrated, id)
	}
	slices.Sort(migrated)
	if d := diffIDs(migrated, fx.okIDs()); d != "" {
		t.Fatalf("migrated IDs: %s", d)
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Fatalf("failed IDs: %s", d)
	}
	if st := stateMap(t, fx.DB); st[splitKKey] != 4 {
		t.Fatalf("split_k = %d, want 4", st[splitKKey])
	}
}

func TestRunRangesFatalErrorNamesRange(t *testing.T) {
	for _, tc := range []struct {
		failRange int
		wantUpper func(st map[string]int64) string
	}{
		{2, func(st map[string]int64) string { return fmt.Sprint(st[rangeUpperKey(2)]) }},
		{3, func(map[string]int64) string { return "+inf" }},
	} {
		t.Run(fmt.Sprintf("range%d", tc.failRange), func(t *testing.T) {
			fx := newFixture(t)
			seedFourRangeFixture(fx)

			injected := errors.New("injected fatal error")
			setTestHook(t, &testHookBeforeBatchCommit, func(rangeIdx int) error {
				if rangeIdx == tc.failRange {
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

			st := stateMap(t, fx.DB)
			want := fmt.Sprintf("range %d [%d, %s]: ", tc.failRange, st[rangeLowerKey(tc.failRange)], tc.wantUpper(st))
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want prefix %q", err, want)
			}
			if !strings.Contains(out, "Error: "+want) {
				t.Errorf("output lacks %q:\n%s", "Error: "+want, out)
			}
			// the failing range's first batch never committed
			if wm, lower := st[rangeWatermarkKey(tc.failRange)], st[rangeLowerKey(tc.failRange)]; wm != lower-1 {
				t.Errorf("range %d watermark = %d, want %d", tc.failRange, wm, lower-1)
			}
		})
	}
}

func TestFailedModeLeavesStateUntouched(t *testing.T) {
	fx := newFixture(t)
	seedFourRangeFixture(fx)
	ctx := context.Background()

	if out, err := fx.run(ctx, "-b", "2", "-w", "4"); err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if len(failedIDs(t, fx.DB)) == 0 {
		t.Fatal("fixture produced no failed rows")
	}

	before := stateRows(t, fx.DB)
	if out, err := fx.run(ctx, "-b", "2", "-w", "8", "--failed"); err != nil {
		t.Fatalf("--failed run failed: %v\n%s", err, out)
	}
	if after := stateRows(t, fx.DB); !reflect.DeepEqual(before, after) {
		t.Errorf("--failed changed the state table:\nbefore %v\nafter  %v", before, after)
	}
}
