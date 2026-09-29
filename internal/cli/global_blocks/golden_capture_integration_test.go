package global_blocks

// Golden baseline capture (T-011). TestCaptureGoldenBaseline is driven by
// scripts/capture_baseline.sh; it is skipped unless GB_CAPTURE_GOLDEN_BIN
// points at a build of the pre-change tool and GB_MIGRATION_TEST_DSN is set.

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

const (
	captureGoldenBinEnv = "GB_CAPTURE_GOLDEN_BIN"
	goldenBaseCommit    = "7c5eea1"
)

// runBaselineBinary runs the pre-change tool as a subprocess:
// "<bin> global_blocks -d <dsn> <args...>". Its progress output is ignored
// except in failure messages.
func runBaselineBinary(t *testing.T, bin, dsn string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	full := append([]string{"global_blocks", "-d", dsn}, args...)
	out, err := exec.CommandContext(ctx, bin, full...).CombinedOutput()
	if err != nil {
		t.Fatalf("baseline tool %v: %v\noutput:\n%s", args, err, out)
	}
}

func TestCaptureGoldenBaseline(t *testing.T) {
	bin := os.Getenv(captureGoldenBinEnv)
	if bin == "" {
		t.Skipf("%s not set; run scripts/capture_baseline.sh to capture the golden baseline", captureGoldenBinEnv)
	}

	t.Run(goldenNormal, func(t *testing.T) {
		fx := newFixture(t)
		seedGoldenFixture(fx)

		runBaselineBinary(t, bin, fx.DSN, goldenRunArgs()...)
		snap := captureGolden(t, fx.DB)
		checkGoldenClassification(t, fx, snap)

		writeGolden(t, goldenNormal, goldenData{
			Scenario: goldenNormal,
			Description: "fresh seedGoldenFixture, empty migration state; " +
				"one run of the tool with args, without --failed",
			BaseCommit:     goldenBaseCommit,
			Args:           goldenRunArgs(),
			goldenSnapshot: snap,
		})
	})

	t.Run(goldenFailed, func(t *testing.T) {
		fx := newFixture(t)
		seedGoldenFixture(fx)

		runBaselineBinary(t, bin, fx.DSN, goldenRunArgs()...)
		checkGoldenClassification(t, fx, captureGolden(t, fx.DB))

		applyGoldenFailedFixes(fx)
		before := normaliseFailedRows(t, fx.DB, failedRows(t, fx.DB))

		runBaselineBinary(t, bin, fx.DSN, goldenFailedRunArgs()...)
		snap := captureGolden(t, fx.DB)
		// the repaired blocks are now migrated, the rest still fail
		checkGoldenClassification(t, fx, snap)

		writeGolden(t, goldenFailed, goldenData{
			Scenario: goldenFailed,
			Description: "fresh seedGoldenFixture, empty migration state; run the tool with " +
				"goldenRunArgs (no --failed), then applyGoldenFailedFixes, then run the tool with args " +
				"(--failed). failed_before is the failed table right before the --failed run",
			BaseCommit:     goldenBaseCommit,
			Args:           goldenFailedRunArgs(),
			FailedBefore:   before,
			goldenSnapshot: snap,
		})
	})
}

// checkGoldenClassification asserts the baseline tool migrated exactly the
// fixture's OK blocks (once each) and failed exactly its failing blocks, so a
// golden never silently records a fixture that does not behave as designed.
func checkGoldenClassification(t *testing.T, fx *fixture, snap goldenSnapshot) {
	t.Helper()
	var migrated, failed []int64
	for _, b := range snap.Migrated {
		migrated = append(migrated, b.DataID)
	}
	for _, r := range snap.Failed {
		failed = append(failed, r.DataID)
	}
	if d := diffIDs(migrated, fx.okIDs()); d != "" {
		t.Fatalf("baseline migrated IDs differ from fixture OK IDs: %s", d)
	}
	if d := diffIDs(failed, fx.failingIDs()); d != "" {
		t.Fatalf("baseline failed IDs differ from fixture failing IDs: %s", d)
	}
}

// TestGoldenFilesLoad checks the committed golden files decode and describe
// the fixture's shape. No database needed.
func TestGoldenFilesLoad(t *testing.T) {
	normal := loadGolden(t, goldenNormal)
	failed := loadGolden(t, goldenFailed)

	if normal.Scenario != goldenNormal || failed.Scenario != goldenFailed {
		t.Fatalf("scenarios = %q, %q", normal.Scenario, failed.Scenario)
	}
	if normal.BaseCommit != goldenBaseCommit || failed.BaseCommit != goldenBaseCommit {
		t.Errorf("base commits = %q, %q, want %q", normal.BaseCommit, failed.BaseCommit, goldenBaseCommit)
	}
	if !slices.Equal(normal.Args, goldenRunArgs()) || !slices.Equal(failed.Args, goldenFailedRunArgs()) {
		t.Errorf("args = %v, %v; want %v, %v", normal.Args, failed.Args, goldenRunArgs(), goldenFailedRunArgs())
	}

	const okBlocks, failingBlocks, fixed = 51, 9, 3
	if len(normal.Migrated) != okBlocks || len(normal.Failed) != failingBlocks {
		t.Errorf("normal golden: %d migrated, %d failed; want %d, %d",
			len(normal.Migrated), len(normal.Failed), okBlocks, failingBlocks)
	}
	if len(failed.FailedBefore) != failingBlocks {
		t.Errorf("failed golden: %d failed before, want %d", len(failed.FailedBefore), failingBlocks)
	}
	if len(failed.Migrated) != okBlocks+fixed || len(failed.Failed) != failingBlocks-fixed {
		t.Errorf("failed golden: %d migrated, %d failed; want %d, %d",
			len(failed.Migrated), len(failed.Failed), okBlocks+fixed, failingBlocks-fixed)
	}
	// the failed scenario starts from the normal scenario's outcome
	if !slices.Equal(failed.FailedBefore, normal.Failed) {
		t.Errorf("failed golden failed_before differs from normal golden failed table")
	}
}
