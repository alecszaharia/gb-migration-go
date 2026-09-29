package global_blocks

// Integration tests of the aggregate progress output of a full K=4 run
// (T-029). Criteria PE R5 c1–c6.

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// progressLineRe matches one full progress write, prefix included.
var progressLineRe = regexp.MustCompile(`^\x1b\[F\x1b\[2KProgress: (\d+\.\d{2})% \((\d+)/(\d+)\) \| (\d+\.\d{2}) blocks/s$`)

// progressHeaderPrefixes are the only non-progress lines a normal run prints.
var progressHeaderPrefixes = []string{
	"Getting node and metafield ids: OK",
	"Preparing statements: OK",
	"Preparing migration state.. OK",
	"Getting total counts..",
	"Starting migration. Global blocks: ",
	"Batch size: ",
	"Ranges: ",
	"Starting..",
}

// progressLine is one parsed progress write.
type progressLine struct {
	percent   float64
	processed int64
	total     int64
	rate      float64
}

// parseProgressOutput checks the structure of a successful run's stdout and
// returns every progress write in order:
//   - every line containing "Progress:" is a complete progress write starting
//     with the cursor-up + clear-line prefix;
//   - every other line is a known header line or the single empty line the
//     progress reporter reserves (so there are no per-worker/per-batch lines);
//   - progress writes only follow the reserved line and nothing follows them.
func parseProgressOutput(t *testing.T, out string) []progressLine {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("output does not end with a newline:\n%q", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	var (
		progress []progressLine
		reserved = -1
	)
	for i, line := range lines {
		if strings.Contains(line, "Progress:") {
			if !strings.HasPrefix(line, progressLinePrefix) {
				t.Errorf("line %d: progress write without cursor-up/clear prefix: %q", i, line)
				continue
			}
			m := progressLineRe.FindStringSubmatch(line)
			if m == nil {
				t.Errorf("line %d: malformed progress line: %q", i, line)
				continue
			}
			if reserved < 0 {
				t.Errorf("line %d: progress write before the reserved line", i)
			}
			pct, _ := strconv.ParseFloat(m[1], 64)
			processed, _ := strconv.ParseInt(m[2], 10, 64)
			total, _ := strconv.ParseInt(m[3], 10, 64)
			rate, _ := strconv.ParseFloat(m[4], 64)
			progress = append(progress, progressLine{pct, processed, total, rate})
			continue
		}
		if len(progress) > 0 {
			t.Errorf("line %d: non-progress line after progress output: %q", i, line)
			continue
		}
		if line == "" {
			if reserved >= 0 {
				t.Errorf("line %d: more than one reserved progress line", i)
			}
			reserved = i
			continue
		}
		known := false
		for _, p := range progressHeaderPrefixes {
			if strings.HasPrefix(line, p) {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("line %d: unexpected line (per-worker/per-batch output?): %q", i, line)
		}
	}
	if len(progress) == 0 {
		t.Fatalf("no progress line in output:\n%q", out)
	}
	return progress
}

// checkProgressLines checks that all writes share wantTotal, that processed
// never decreases, and that the final write shows wantFinal processed with a
// consistent percentage and a positive rate.
func checkProgressLines(t *testing.T, lines []progressLine, wantTotal, wantFinal int64) {
	t.Helper()
	var prev int64
	for i, l := range lines {
		if l.total != wantTotal {
			t.Errorf("progress write %d: total = %d, want %d", i, l.total, wantTotal)
		}
		if l.processed < prev {
			t.Errorf("progress write %d: processed went back from %d to %d", i, prev, l.processed)
		}
		prev = l.processed
	}

	final := lines[len(lines)-1]
	if final.processed != wantFinal {
		t.Errorf("final processed = %d, want %d", final.processed, wantFinal)
	}
	// PE R5 c6
	if final.processed < final.total {
		t.Errorf("final processed %d < total %d", final.processed, final.total)
	}
	wantPct := strconv.FormatFloat(float64(final.processed)/float64(final.total)*100, 'f', 2, 64)
	if got := strconv.FormatFloat(final.percent, 'f', 2, 64); got != wantPct {
		t.Errorf("final percent = %s, want %s", got, wantPct)
	}
	if final.rate <= 0 {
		t.Errorf("final rate = %.2f blocks/s, want > 0", final.rate)
	}
}

// seedProgressFixture seeds 40 eligible blocks (4 failing) and 2 ineligible
// ones, so a K=4 split at -b 2 gives every range several batches.
func seedProgressFixture(fx *fixture) {
	fx.addEligibleN(9)
	fx.addFailing(failMalformedRules)
	fx.addWrongNode()
	fx.addEligibleN(9)
	fx.addFailing(failInvalidIRI)
	fx.addEligibleN(9)
	fx.addFailing(failMissingCollectionType)
	fx.addLegacyAPI()
	fx.addEligibleN(9)
	fx.addFailing(failMalformedRules)
}

// slowBatches delays every batch start so a run lasts longer than the render
// interval and the ticker gets a chance to fire; assertions never depend on
// it. extra, when not nil, runs first.
func slowBatches(t *testing.T, extra func(rangeIdx int) error) {
	t.Helper()
	setTestHook(t, &testHookBatchStart, func(rangeIdx int) error {
		if extra != nil {
			if err := extra(rangeIdx); err != nil {
				return err
			}
		}
		time.Sleep(60 * time.Millisecond)
		return nil
	})
}

// remainingTotal is Σ getRemainingCountInRange over the stored split.
func remainingTotal(t *testing.T, fx *fixture) (int64, int) {
	t.Helper()
	ctx := context.Background()
	repo, err := newPrepareRepository(ctx, fx.DB)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	migSt := &state{}
	if err := migSt.init(ctx, fx.DB); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	_, ranges, ok, err := migSt.loadSplit(ctx)
	if err != nil || !ok {
		t.Fatalf("loadSplit: ok=%v err=%v", ok, err)
	}
	var total int64
	for _, r := range ranges {
		n, err := repo.getRemainingCountInRange(ctx, r.Watermark, r.Upper)
		if err != nil {
			t.Fatalf("getRemainingCountInRange(range %d): %v", r.Index, err)
		}
		total += n
	}
	return total, len(ranges)
}

// TestProgressOutputFreshRun: a fresh K=4 run prints exactly one progress
// line whose total is every eligible block and whose final processed count
// equals it (PE R5 c1–c6).
func TestProgressOutputFreshRun(t *testing.T) {
	fx := newFixture(t)
	seedProgressFixture(fx)
	want := int64(len(fx.eligibleIDs()))
	slowBatches(t, nil)

	out, err := fx.run(context.Background(), "-b", "2", "-w", "4")
	if err != nil {
		t.Fatalf("run failed: %v\n%q", err, out)
	}

	lines := parseProgressOutput(t, out)
	checkProgressLines(t, lines, want, want)
	if !strings.Contains(out, "Starting migration. Global blocks:  "+strconv.FormatInt(want, 10)+"\n") {
		t.Errorf("header does not announce %d blocks:\n%q", want, out)
	}
	if !strings.Contains(out, "Ranges:  4\n") {
		t.Errorf("header does not announce 4 ranges:\n%q", out)
	}
	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("migrated IDs: %s", d)
	}
}

// TestProgressOutputResumedRunTotal: on a resumed run the total is the sum of
// the work remaining after each range's stored watermark, not the eligible
// count (PE R5 c5).
func TestProgressOutputResumedRunTotal(t *testing.T) {
	fx := newFixture(t)
	seedProgressFixture(fx)
	eligible := int64(len(fx.eligibleIDs()))
	ctx := context.Background()

	// Store a K=4 split and fully migrate range 0 plus one batch of range 2.
	repo, err := newPrepareRepository(ctx, fx.DB)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	migSt := &state{}
	if err := migSt.init(ctx, fx.DB); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	ranges, err := ensureSplit(ctx, fx.DB, migSt, repo, 4)
	if err != nil || len(ranges) != 4 {
		t.Fatalf("ensureSplit: %d ranges, err %v", len(ranges), err)
	}
	if err := runRange(ctx, migSt, repo, ranges[0], 2, nil); err != nil {
		t.Fatalf("runRange(0): %v", err)
	}
	batches := 0
	setTestHook(t, &testHookBatchStart, func(int) error {
		batches++
		if batches > 1 {
			return context.Canceled
		}
		return nil
	})
	if err := runRange(ctx, migSt, repo, ranges[2], 2, nil); err == nil {
		t.Fatal("runRange(2) was not stopped after one batch")
	}

	want, k := remainingTotal(t, fx)
	if k != 4 {
		t.Fatalf("stored split has %d ranges, want 4", k)
	}
	if want <= 0 || want >= eligible {
		t.Fatalf("remaining total %d, want 0 < total < %d (partial migration)", want, eligible)
	}

	slowBatches(t, nil)
	out, err := fx.run(ctx, "-b", "2", "-w", "4")
	if err != nil {
		t.Fatalf("run failed: %v\n%q", err, out)
	}
	lines := parseProgressOutput(t, out)
	checkProgressLines(t, lines, want, want)
	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("migrated IDs: %s", d)
	}
}

// TestProgressOutputCountsBlockInsertedDuringRun: a block inserted into the
// open-ended last range during the run is migrated and counted, so the final
// processed count is total + 1 (PE R5 c6).
func TestProgressOutputCountsBlockInsertedDuringRun(t *testing.T) {
	fx := newFixture(t)
	seedProgressFixture(fx)
	want := int64(len(fx.eligibleIDs()))
	lateID := fx.maxSeededID() + 1000
	const lastRange = 3

	var once sync.Once
	slowBatches(t, func(rangeIdx int) error {
		if rangeIdx == lastRange {
			once.Do(func() { fx.addEligible(lateID) })
		}
		return nil
	})

	out, err := fx.run(context.Background(), "-b", "2", "-w", "4")
	if err != nil {
		t.Fatalf("run failed: %v\n%q", err, out)
	}
	if _, ok := fx.blocks[lateID]; !ok {
		t.Fatalf("late block %d was never inserted", lateID)
	}

	lines := parseProgressOutput(t, out)
	checkProgressLines(t, lines, want, want+1)
	if n := migratedCountByDataID(t, fx.DB)[lateID]; n != 1 {
		t.Errorf("late block %d migrated %d times, want 1", lateID, n)
	}
	if d := diffIDs(migratedDataIDs(t, fx.DB), fx.okIDs()); d != "" {
		t.Errorf("migrated IDs: %s", d)
	}
}
