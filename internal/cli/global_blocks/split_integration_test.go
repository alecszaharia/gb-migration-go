package global_blocks

// T-015: integration tests for range split creation and persistence
// (RS R1 c1–c3, c6–c9; RS R2 c1–c4).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// newSplitEnv prepares the state and repository ensureSplit needs, exactly as
// migrationRun does.
func newSplitEnv(t *testing.T, db *sql.DB) (*state, *repository) {
	t.Helper()
	repo, err := newPrepareRepository(context.Background(), db)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	return newTestState(t, db), repo
}

// runEnsureSplit calls ensureSplit with stdout captured.
func runEnsureSplit(t *testing.T, ctx context.Context, db *sql.DB, st *state, repo *repository, k int) ([]storedRange, string, error) {
	t.Helper()
	var (
		ranges []storedRange
		err    error
	)
	out := captureStdout(t, func() { ranges, err = ensureSplit(ctx, db, st, repo, k) })
	return ranges, out, err
}

// clearState deletes every state row so the next ensureSplit computes anew.
func clearState(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), "DELETE FROM "+stateTable); err != nil {
		t.Fatalf("clear state: %v", err)
	}
}

// assertNoMigratedData fails when any migrated table has rows.
func assertNoMigratedData(t *testing.T, db *sql.DB) {
	t.Helper()
	for table, n := range rowCounts(t, db) {
		if n != 0 {
			t.Fatalf("table %s has %d rows before any worker ran", table, n)
		}
	}
	if ids := migratedDataIDs(t, db); len(ids) != 0 {
		t.Fatalf("migrated data IDs before any worker ran: %v", ids)
	}
}

// storedSplitFromState rebuilds the split straight from the raw state rows
// (independent of loadSplit) and checks the row layout: split_k, a lower and
// watermark per range, an upper for all but the last range, nothing else.
// With freshWatermarks every watermark must still be Lower-1.
func storedSplitFromState(t *testing.T, db *sql.DB, freshWatermarks bool) []idRange {
	t.Helper()
	m := stateMap(t, db)
	kv, ok := m[splitKKey]
	if !ok {
		t.Fatalf("no %s row (state %v)", splitKKey, m)
	}
	k := int(kv)
	want := map[string]bool{splitKKey: true}
	ranges := make([]idRange, 0, k)
	for i := 0; i < k; i++ {
		lower, ok := m[rangeLowerKey(i)]
		if !ok {
			t.Fatalf("missing %s (state %v)", rangeLowerKey(i), m)
		}
		want[rangeLowerKey(i)] = true
		wm, ok := m[rangeWatermarkKey(i)]
		if !ok {
			t.Fatalf("missing %s (state %v)", rangeWatermarkKey(i), m)
		}
		want[rangeWatermarkKey(i)] = true
		if freshWatermarks && wm != lower-1 {
			t.Fatalf("range %d initial watermark = %d, want %d", i, wm, lower-1)
		}
		r := idRange{Index: i, Lower: lower}
		upper, hasUpper := m[rangeUpperKey(i)]
		if i == k-1 {
			if hasUpper {
				t.Fatalf("last range %d has a stored upper bound %d", i, upper)
			}
		} else {
			if !hasUpper {
				t.Fatalf("missing %s (state %v)", rangeUpperKey(i), m)
			}
			want[rangeUpperKey(i)] = true
			r.Upper = i64(upper)
		}
		ranges = append(ranges, r)
	}
	for _, key := range splitKeys(t, db) {
		if !want[key] {
			t.Fatalf("unexpected split key %s (split_k=%d)", key, k)
		}
	}
	return ranges
}

// assertSplitShape checks split_k = number of ranges = min(k, N), coverage,
// disjointness, contiguity and balance of ranges over the eligible IDs, and
// that no ineligible ID is used as a bound.
func assertSplitShape(t *testing.T, ranges []idRange, eligible, ineligible []int64, k int) {
	t.Helper()
	n := len(eligible)
	wantK := min(k, n)
	if len(ranges) != wantK {
		t.Fatalf("stored %d ranges, want min(K=%d, N=%d)=%d", len(ranges), k, n, wantK)
	}

	// coverage and disjointness: every eligible ID in exactly one range
	sizes := make([]int, len(ranges))
	for _, id := range eligible {
		owners := 0
		for i, r := range ranges {
			if r.contains(id) {
				owners++
				sizes[i]++
			}
		}
		if owners != 1 {
			t.Fatalf("eligible ID %d is in %d ranges, want exactly 1", id, owners)
		}
	}

	// contiguity: sorted, each lower above the previous upper, and no eligible
	// ID falls in the gap between two neighbouring ranges
	for i, r := range ranges {
		if r.Index != i {
			t.Fatalf("range at position %d has index %d", i, r.Index)
		}
		if r.Upper != nil && *r.Upper < r.Lower {
			t.Fatalf("range %d upper %d < lower %d", i, *r.Upper, r.Lower)
		}
		if i == 0 {
			continue
		}
		prev := ranges[i-1]
		if prev.Upper == nil {
			t.Fatalf("range %d is open-ended but is not last", i-1)
		}
		if r.Lower <= *prev.Upper {
			t.Fatalf("range %d lower %d <= range %d upper %d", i, r.Lower, i-1, *prev.Upper)
		}
		for _, id := range eligible {
			if id > *prev.Upper && id < r.Lower {
				t.Fatalf("eligible ID %d lies between range %d and range %d", id, i-1, i)
			}
		}
	}
	if last := ranges[len(ranges)-1]; last.Upper != nil {
		t.Fatalf("last range has upper %d, want open-ended", *last.Upper)
	}

	// bounds are real eligible IDs, never ineligible ones
	isEligible := map[int64]bool{}
	for _, id := range eligible {
		isEligible[id] = true
	}
	for i, r := range ranges {
		if !isEligible[r.Lower] {
			t.Fatalf("range %d lower %d is not an eligible ID", i, r.Lower)
		}
		if r.Upper != nil && !isEligible[*r.Upper] {
			t.Fatalf("range %d upper %d is not an eligible ID", i, *r.Upper)
		}
	}
	for _, id := range ineligible {
		for i, r := range ranges {
			if r.Lower == id || (r.Upper != nil && *r.Upper == id) {
				t.Fatalf("ineligible ID %d is a bound of range %d", id, i)
			}
		}
	}

	// balance: sizes within ±1 of N/K; N<K gives single-block ranges
	for i, s := range sizes {
		if n >= k {
			if s != n/k && s != n/k+1 {
				t.Fatalf("range %d has %d eligible IDs, want %d or %d (N=%d K=%d)", i, s, n/k, n/k+1, n, k)
			}
		} else if s != 1 {
			t.Fatalf("range %d has %d eligible IDs, want 1 (N=%d < K=%d)", i, s, n, k)
		}
	}
}

// seedMixed seeds eligible (ok and failing) blocks interleaved with every
// ineligible class, including ineligible rows below the first and above the
// last eligible ID.
func seedMixed(fx *fixture) {
	fx.addWrongNode()
	fx.addLegacyAPI()
	for i := 0; i < 6; i++ {
		fx.addEligibleN(2)
		fx.addWrongNode()
		fx.addEligible()
		fx.addLegacyAPI()
		fx.addFailing(failMalformedRules)
		fx.addNoAPIVersion()
		fx.addEligibleN(2)
	}
	fx.addNoAPIVersion()
	fx.addWrongNode()
}

func TestEnsureSplitCreatesPersistedSplit(t *testing.T) {
	ctx := context.Background()

	mixed := newFixture(t)
	seedMixed(mixed)
	eligible := mixed.eligibleIDs()
	ineligible := mixed.ineligibleIDs()
	if len(ineligible) == 0 || len(eligible) != 36 {
		t.Fatalf("fixture: %d eligible, %d ineligible", len(eligible), len(ineligible))
	}

	// same eligible IDs, no ineligible rows: the split must be identical
	clean := newFixture(t)
	clean.addEligible(eligible...)

	mixedSt, mixedRepo := newSplitEnv(t, mixed.DB)
	cleanSt, cleanRepo := newSplitEnv(t, clean.DB)

	for _, k := range []int{1, 3, 8, len(eligible), len(eligible) + 5} {
		t.Run(fmt.Sprintf("K=%d", k), func(t *testing.T) {
			clearState(t, mixed.DB)
			clearState(t, clean.DB)

			got, _, err := runEnsureSplit(t, ctx, mixed.DB, mixedSt, mixedRepo, k)
			if err != nil {
				t.Fatalf("ensureSplit: %v", err)
			}
			assertNoMigratedData(t, mixed.DB)

			stored := storedSplitFromState(t, mixed.DB, true)
			if m := stateMap(t, mixed.DB); m[splitKKey] != int64(len(stored)) {
				t.Fatalf("split_k = %d, stored ranges = %d", m[splitKKey], len(stored))
			}
			assertSplitShape(t, stored, eligible, ineligible, k)

			// returned ranges match the stored ones, watermarks at Lower-1
			if len(got) != len(stored) {
				t.Fatalf("ensureSplit returned %d ranges, stored %d", len(got), len(stored))
			}
			for i := range got {
				if !reflect.DeepEqual(got[i].idRange, stored[i]) || got[i].Watermark != stored[i].Lower-1 {
					t.Fatalf("returned range %d = %+v, stored %+v", i, got[i], stored[i])
				}
			}

			// loadSplit reads back the same split
			lk, loaded, ok, err := mixedSt.loadSplit(ctx)
			if err != nil || !ok || lk != len(stored) || !reflect.DeepEqual(loaded, got) {
				t.Fatalf("loadSplit k=%d ok=%v err=%v ranges=%+v, want %+v", lk, ok, err, loaded, got)
			}

			// ineligible rows do not change the split
			if _, _, err := runEnsureSplit(t, ctx, clean.DB, cleanSt, cleanRepo, k); err != nil {
				t.Fatalf("ensureSplit (eligible-only fixture): %v", err)
			}
			if want := stateRows(t, clean.DB); !reflect.DeepEqual(stateRows(t, mixed.DB), want) {
				t.Fatalf("split with ineligible rows differs from eligible-only split:\n got %v\nwant %v",
					stateRows(t, mixed.DB), want)
			}
		})
	}
}

func TestEnsureSplitNothingEligible(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.addWrongNode()
	fx.addLegacyAPI()
	fx.addNoAPIVersion()
	fx.addWrongNode()
	st, repo := newSplitEnv(t, fx.DB)

	for _, k := range []int{1, 4} {
		ranges, out, err := runEnsureSplit(t, ctx, fx.DB, st, repo, k)
		if err != nil {
			t.Fatalf("K=%d: ensureSplit: %v", k, err)
		}
		if len(ranges) != 0 {
			t.Fatalf("K=%d: ensureSplit returned %d ranges, want none", k, len(ranges))
		}
		if !strings.Contains(out, "Nothing to migrate") {
			t.Fatalf("K=%d: output lacks 'Nothing to migrate':\n%s", k, out)
		}
		if keys := splitKeys(t, fx.DB); len(keys) != 0 {
			t.Fatalf("K=%d: split rows stored with nothing eligible: %v", k, keys)
		}
		assertNoMigratedData(t, fx.DB)
	}

	// end to end: the command stores no split either
	out, err := fx.run(ctx, "-w", "3")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Nothing to migrate") {
		t.Fatalf("run output lacks 'Nothing to migrate':\n%s", out)
	}
	if keys := splitKeys(t, fx.DB); len(keys) != 0 {
		t.Fatalf("command stored split rows with nothing eligible: %v", keys)
	}
	assertNoMigratedData(t, fx.DB)
}

// TestEnsureSplitTerminationDuringCreation interrupts the split store at every
// step, by cancelling the context and by returning an error, and checks the
// state never holds a partial split and a clean rerun stores the full split.
func TestEnsureSplitTerminationDuringCreation(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	seedMixed(fx)
	eligible, ineligible := fx.eligibleIDs(), fx.ineligibleIDs()
	st, repo := newSplitEnv(t, fx.DB)

	const k = 3
	want := computeSplit(eligible, k)
	steps := splitStoreWrites(want) // hook runs at steps 0..splitStoreWrites (last is pre-commit)
	errInjected := errors.New("injected termination")

	// assertAllOrNothing checks the state holds no split rows at all, or a
	// complete split that loadSplit accepts. It reports whether a split exists.
	assertAllOrNothing := func(t *testing.T) bool {
		t.Helper()
		keys := splitKeys(t, fx.DB)
		_, ranges, ok, err := st.loadSplit(ctx)
		if err != nil {
			t.Fatalf("partial split stored (loadSplit: %v), keys %v", err, keys)
		}
		if !ok {
			if len(keys) != 0 {
				t.Fatalf("split rows without split_k: %v", keys)
			}
			return false
		}
		stored := storedSplitFromState(t, fx.DB, true)
		if len(ranges) != len(stored) {
			t.Fatalf("loadSplit %d ranges, stored %d", len(ranges), len(stored))
		}
		assertSplitShape(t, stored, eligible, ineligible, k)
		return true
	}

	variants := []struct {
		name   string
		inject func(cancel context.CancelFunc) error
	}{
		{"cancel", func(cancel context.CancelFunc) error { cancel(); return nil }},
		{"error", func(context.CancelFunc) error { return errInjected }},
	}

	for _, v := range variants {
		for i := 0; i <= steps; i++ {
			t.Run(fmt.Sprintf("%s/step=%d", v.name, i), func(t *testing.T) {
				clearState(t, fx.DB)

				cctx, cancel := context.WithCancel(ctx)
				defer cancel()
				hits := 0
				setTestHook(t, &testHookSplitStoreStep, func(step int) error {
					if step == i {
						hits++
						return v.inject(cancel)
					}
					return nil
				})

				ranges, _, err := runEnsureSplit(t, cctx, fx.DB, st, repo, k)
				if hits != 1 {
					t.Fatalf("hook reached step %d %d times, want 1", i, hits)
				}
				if err == nil {
					t.Fatalf("ensureSplit succeeded after termination at step %d (ranges %d)", i, len(ranges))
				}
				if v.name == "error" && !errors.Is(err, errInjected) {
					t.Fatalf("ensureSplit err = %v, want injected error", err)
				}
				if assertAllOrNothing(t) {
					t.Logf("step %d: full split present after termination", i)
				}
				assertNoMigratedData(t, fx.DB)

				// clean rerun stores the full split
				testHookSplitStoreStep = nil
				got, _, err := runEnsureSplit(t, ctx, fx.DB, st, repo, k)
				if err != nil {
					t.Fatalf("clean rerun: %v", err)
				}
				if !assertAllOrNothing(t) {
					t.Fatal("clean rerun stored no split")
				}
				if stored := storedSplitFromState(t, fx.DB, true); !reflect.DeepEqual(stored, want) {
					t.Fatalf("rerun split = %+v, want %+v", stored, want)
				}
				if len(got) != len(want) {
					t.Fatalf("rerun returned %d ranges, want %d", len(got), len(want))
				}
				assertNoMigratedData(t, fx.DB)
			})
		}
	}
}

// TestSplitStoredBeforeFirstBatch runs the command with -w 3 and checks, at the
// first batch start, that the full split is already stored and nothing has
// been migrated yet.
func TestSplitStoredBeforeFirstBatch(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	seedMixed(fx)
	eligible, ineligible := fx.eligibleIDs(), fx.ineligibleIDs()

	var (
		once     sync.Once
		mu       sync.Mutex
		seen     bool
		hookErr  error
		stateAt  map[string]int64
		countsAt map[string]int64
	)
	setTestHook(t, &testHookBatchStart, func(int) error {
		once.Do(func() {
			// may run on a worker goroutine: record, assert later
			st, counts, err := rawStateAndCounts(fx.DB)
			mu.Lock()
			seen, stateAt, countsAt, hookErr = true, st, counts, err
			mu.Unlock()
		})
		return nil
	})

	out, err := fx.run(ctx, "-b", "4", "-w", "3")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}

	mu.Lock()
	defer mu.Unlock()
	if !seen {
		t.Fatal("testHookBatchStart never ran")
	}
	if hookErr != nil {
		t.Fatalf("query in hook: %v", hookErr)
	}
	for table, n := range countsAt {
		if n != 0 {
			t.Fatalf("at first batch start %s already has %d rows", table, n)
		}
	}
	if stateAt[splitKKey] != 3 {
		t.Fatalf("at first batch start split_k = %d, want 3 (state %v)", stateAt[splitKKey], stateAt)
	}
	// every bound of the final split was already present at first batch start
	final := storedSplitFromState(t, fx.DB, false)
	assertSplitShape(t, final, eligible, ineligible, 3)
	for i, r := range final {
		if v, ok := stateAt[rangeLowerKey(i)]; !ok || v != r.Lower {
			t.Fatalf("at first batch start %s = %d (present %v), want %d", rangeLowerKey(i), v, ok, r.Lower)
		}
		if _, ok := stateAt[rangeWatermarkKey(i)]; !ok {
			t.Fatalf("at first batch start %s missing", rangeWatermarkKey(i))
		}
		v, ok := stateAt[rangeUpperKey(i)]
		if r.Upper == nil {
			if ok {
				t.Fatalf("at first batch start last range has %s", rangeUpperKey(i))
			}
		} else if !ok || v != *r.Upper {
			t.Fatalf("at first batch start %s = %d (present %v), want %d", rangeUpperKey(i), v, ok, *r.Upper)
		}
	}
}

// rawStateAndCounts reads the state table and migrated table row counts
// without testing.T, so it is safe on any goroutine.
func rawStateAndCounts(db *sql.DB) (map[string]int64, map[string]int64, error) {
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, "SELECT set_key, value FROM "+stateTable)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	st := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, nil, err
		}
		st[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	counts := map[string]int64{}
	for _, table := range migratedTables {
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n); err != nil {
			return nil, nil, err
		}
		counts[table] = n
	}
	return st, counts, nil
}
