package global_blocks

// Golden equivalence with the pre-change sequential tool (T-028, PE R6 c5 and
// PE R7 c4). The goldens in testdata/golden were recorded by the pre-change
// tool (T-011, see golden_helpers_test.go); these tests replay the exact same
// scenarios with the new tool and compare the normalised outcome.

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

// TestGoldenEquivalenceNormal: from empty migration state, a normal run of
// the new tool with -w 1 (PE R6 c5) matches the pre-change tool. -w 4 and
// -w 8 are extra evidence that K does not change the outcome.
func TestGoldenEquivalenceNormal(t *testing.T) {
	want := loadGolden(t, goldenNormal)
	for _, k := range []int{1, 4, 8} {
		t.Run(fmt.Sprintf("K=%d", k), func(t *testing.T) {
			fx := newFixture(t)
			seedGoldenFixture(fx)
			if st := stateRows(t, fx.DB); len(st) != 0 {
				t.Fatalf("migration state not empty before the run: %v", st)
			}

			args := append(goldenRunArgs(), "-w", fmt.Sprint(k))
			if out, err := fx.run(context.Background(), args...); err != nil {
				t.Fatalf("run %v failed: %v\n%s", args, err, out)
			}
			assertGoldenEqual(t, goldenNormal, want.goldenSnapshot, captureGolden(t, fx.DB))
		})
	}
}

// TestGoldenEquivalenceFailed replays the golden failed scenario: normal run,
// applyGoldenFailedFixes, then --failed (PE R7 c4). The failed table right
// before the --failed run must equal the golden's failed_before, so both tools
// start the --failed run from the same input. The normal run's -w and the
// --failed run's -w are varied: --failed ignores the worker count, and its
// outcome must not depend on how the preceding normal run was split.
func TestGoldenEquivalenceFailed(t *testing.T) {
	want := loadGolden(t, goldenFailed)
	for _, tc := range []struct{ normalK, failedK int }{
		{1, 1},
		{4, 8},
	} {
		t.Run(fmt.Sprintf("normalK=%d/failedK=%d", tc.normalK, tc.failedK), func(t *testing.T) {
			ctx := context.Background()
			fx := newFixture(t)
			seedGoldenFixture(fx)

			args := append(goldenRunArgs(), "-w", fmt.Sprint(tc.normalK))
			if out, err := fx.run(ctx, args...); err != nil {
				t.Fatalf("normal run %v failed: %v\n%s", args, err, out)
			}
			applyGoldenFailedFixes(fx)

			before := normaliseFailedRows(t, fx.DB, failedRows(t, fx.DB))
			if !reflect.DeepEqual(before, want.FailedBefore) {
				t.Fatalf("failed table before --failed differs from golden failed_before:\n  want %+v\n  got  %+v",
					want.FailedBefore, before)
			}
			stateBefore := stateRows(t, fx.DB)

			args = append(goldenFailedRunArgs(), "-w", fmt.Sprint(tc.failedK))
			if out, err := fx.run(ctx, args...); err != nil {
				t.Fatalf("--failed run %v failed: %v\n%s", args, err, out)
			}
			assertGoldenEqual(t, goldenFailed, want.goldenSnapshot, captureGolden(t, fx.DB))
			if after := stateRows(t, fx.DB); !reflect.DeepEqual(stateBefore, after) {
				t.Errorf("--failed changed the state table:\nbefore %v\nafter  %v", stateBefore, after)
			}
		})
	}
}
