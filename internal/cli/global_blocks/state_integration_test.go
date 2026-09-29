package global_blocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// newTestState runs state.init against db and returns the ready state.
func newTestState(t *testing.T, db *sql.DB) *state {
	t.Helper()
	st := &state{}
	if err := st.init(context.Background(), db); err != nil {
		t.Fatalf("state.init: %v", err)
	}
	return st
}

// splitKeys returns the split_k and range_* keys present in the state table.
func splitKeys(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var keys []string
	for k := range stateMap(t, db) {
		if k == splitKKey || rangeKeyRe.MatchString(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

type stateColumnLayout struct {
	KeyType   string
	ValueType string
}

func stateColumns(t *testing.T, db *sql.DB) stateColumnLayout {
	t.Helper()
	var l stateColumnLayout
	err := db.QueryRowContext(context.Background(), `SELECT
			MAX(CASE WHEN COLUMN_NAME = 'set_key' THEN COLUMN_TYPE END),
			MAX(CASE WHEN COLUMN_NAME = 'value' THEN COLUMN_TYPE END)
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, stateTable).Scan(&l.KeyType, &l.ValueType)
	if err != nil {
		t.Fatalf("read state column layout: %v", err)
	}
	return l
}

func assertCurrentStateLayout(t *testing.T, db *sql.DB) {
	t.Helper()
	l := stateColumns(t, db)
	if l.KeyType != "varchar(64)" || !strings.HasPrefix(l.ValueType, "bigint") {
		t.Fatalf("state columns = %+v, want set_key varchar(64) and value bigint", l)
	}
}

// ---- T-009: split persistence ----

func TestStateStoreLoadSplitRoundTrip(t *testing.T) {
	_, db := requireDB(t)
	ctx := context.Background()

	ids := make([]int64, 0, 40)
	for i := int64(0); i < 40; i++ {
		ids = append(ids, 100+i*3)
	}

	for _, k := range []int{1, 3, 8} {
		t.Run(fmt.Sprintf("K=%d", k), func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+stateTable); err != nil {
				t.Fatalf("drop state: %v", err)
			}
			st := newTestState(t, db)
			if _, _, ok, err := st.loadSplit(ctx); err != nil || ok {
				t.Fatalf("empty state: loadSplit ok=%v err=%v", ok, err)
			}

			split := computeSplit(ids, k)
			if err := st.storeSplit(ctx, db, split); err != nil {
				t.Fatalf("storeSplit: %v", err)
			}

			m := stateMap(t, db)
			if m[splitKKey] != int64(k) {
				t.Fatalf("split_k = %d, want %d", m[splitKKey], k)
			}
			if got := len(splitKeys(t, db)); got != splitStoreWrites(split) {
				t.Fatalf("stored %d split rows, want %d", got, splitStoreWrites(split))
			}
			if _, has := m[rangeUpperKey(k-1)]; has {
				t.Fatalf("last range has a stored upper bound")
			}

			gotK, ranges, ok, err := st.loadSplit(ctx)
			if err != nil || !ok {
				t.Fatalf("loadSplit ok=%v err=%v", ok, err)
			}
			if gotK != k || len(ranges) != k {
				t.Fatalf("loadSplit k=%d ranges=%d, want %d", gotK, len(ranges), k)
			}
			for i, r := range ranges {
				want := split[i]
				if !reflect.DeepEqual(r.idRange, want) {
					t.Fatalf("range %d = %+v (upper %v), want %+v (upper %v)", i, r.idRange, r.Upper, want, want.Upper)
				}
				if r.Watermark != want.Lower-1 {
					t.Fatalf("range %d watermark = %d, want %d", i, r.Watermark, want.Lower-1)
				}
			}
			if ranges[k-1].Upper != nil {
				t.Fatalf("last range upper = %d, want nil", *ranges[k-1].Upper)
			}
		})
	}
}

func TestStateLoadSplitIgnoresLegacyWatermark(t *testing.T) {
	_, db := requireDB(t)
	ctx := context.Background()
	st := newTestState(t, db)

	if err := st.updateState(ctx, 555); err != nil {
		t.Fatalf("updateState: %v", err)
	}
	if _, _, ok, err := st.loadSplit(ctx); err != nil || ok {
		t.Fatalf("legacy-only state: loadSplit ok=%v err=%v", ok, err)
	}

	if err := st.storeSplit(ctx, db, computeSplit([]int64{1, 2, 3, 4}, 2)); err != nil {
		t.Fatalf("storeSplit: %v", err)
	}
	_, ranges, ok, err := st.loadSplit(ctx)
	if err != nil || !ok || len(ranges) != 2 {
		t.Fatalf("loadSplit ranges=%d ok=%v err=%v", len(ranges), ok, err)
	}
	if ranges[0].Watermark != 0 || ranges[1].Watermark != 2 {
		t.Fatalf("watermarks = %d,%d, want 0,2 (legacy key must be ignored)", ranges[0].Watermark, ranges[1].Watermark)
	}
}

func TestStateStoreSplitAllOrNothing(t *testing.T) {
	_, db := requireDB(t)
	ctx := context.Background()
	st := newTestState(t, db)
	if err := st.updateState(ctx, 77); err != nil {
		t.Fatalf("updateState: %v", err)
	}
	before := stateRows(t, db)

	split := computeSplit([]int64{10, 20, 30, 40, 50, 60, 70}, 3)
	steps := splitStoreWrites(split) + 1 // hook runs before each write and before commit

	errInjected := errors.New("injected split store failure")
	for i := 0; i < steps; i++ {
		setTestHook(t, &testHookSplitStoreStep, func(step int) error {
			if step == i {
				return errInjected
			}
			return nil
		})
		err := st.storeSplit(ctx, db, split)
		if !errors.Is(err, errInjected) {
			t.Fatalf("step %d: storeSplit err = %v, want injected error", i, err)
		}
		if got := stateRows(t, db); !reflect.DeepEqual(got, before) {
			t.Fatalf("step %d: state changed after failed store:\n got %v\nwant %v", i, got, before)
		}
	}

	for i := 0; i < steps; i++ {
		cctx, cancel := context.WithCancel(ctx)
		setTestHook(t, &testHookSplitStoreStep, func(step int) error {
			if step == i {
				cancel()
			}
			return nil
		})
		err := st.storeSplit(cctx, db, split)
		cancel()
		if err == nil {
			t.Fatalf("step %d: storeSplit succeeded after context cancel", i)
		}
		if got := stateRows(t, db); !reflect.DeepEqual(got, before) {
			t.Fatalf("step %d: state changed after cancelled store:\n got %v\nwant %v", i, got, before)
		}
	}

	calls := 0
	setTestHook(t, &testHookSplitStoreStep, func(int) error { calls++; return nil })
	if err := st.storeSplit(ctx, db, split); err != nil {
		t.Fatalf("storeSplit: %v", err)
	}
	if calls != steps {
		t.Fatalf("hook ran %d times, want %d", calls, steps)
	}
	if k, _, ok, err := st.loadSplit(ctx); err != nil || !ok || k != 3 {
		t.Fatalf("after successful store: k=%d ok=%v err=%v", k, ok, err)
	}
}

func TestStateUpdateRangeWatermarkInTx(t *testing.T) {
	_, db := requireDB(t)
	ctx := context.Background()
	st := newTestState(t, db)
	if err := st.storeSplit(ctx, db, computeSplit([]int64{1, 2, 3, 4, 5, 6, 7, 8, 9}, 3)); err != nil {
		t.Fatalf("storeSplit: %v", err)
	}
	before := stateMap(t, db)

	// Rolled back: nothing changes.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := st.withTx(ctx, tx).updateRangeWatermark(ctx, 1, 5); err != nil {
		t.Fatalf("updateRangeWatermark: %v", err)
	}
	if got := stateMap(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("uncommitted watermark visible outside tx: %v", got)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := stateMap(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("state changed after rollback: %v", got)
	}

	// Committed: only range 1's watermark changes.
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := st.withTx(ctx, tx).updateRangeWatermark(ctx, 1, 5_000_000_000); err != nil {
		t.Fatalf("updateRangeWatermark: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	want := map[string]int64{}
	for k, v := range before {
		want[k] = v
	}
	want[rangeWatermarkKey(1)] = 5_000_000_000
	if got := stateMap(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("state after commit:\n got %v\nwant %v", got, want)
	}

	_, ranges, _, err := st.loadSplit(ctx)
	if err != nil {
		t.Fatalf("loadSplit: %v", err)
	}
	if ranges[0].Watermark != before[rangeWatermarkKey(0)] || ranges[1].Watermark != 5_000_000_000 ||
		ranges[2].Watermark != before[rangeWatermarkKey(2)] {
		t.Fatalf("loaded watermarks = %d,%d,%d", ranges[0].Watermark, ranges[1].Watermark, ranges[2].Watermark)
	}
}

func TestStateLoadSplitRejectsInconsistentRows(t *testing.T) {
	cases := map[string]string{
		"split_k exceeds ranges": "DELETE FROM " + stateTable + " WHERE set_key LIKE 'range\\_2\\_%'",
		"missing watermark":      "DELETE FROM " + stateTable + " WHERE set_key = 'range_1_watermark'",
		"missing lower":          "DELETE FROM " + stateTable + " WHERE set_key = 'range_0_lower'",
		"extra range":            "INSERT INTO " + stateTable + " (set_key, value) VALUES ('range_3_lower', 99), ('range_3_watermark', 98)",
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			_, db := requireDB(t)
			ctx := context.Background()
			st := newTestState(t, db)
			if err := st.storeSplit(ctx, db, computeSplit([]int64{1, 2, 3, 4, 5, 6}, 3)); err != nil {
				t.Fatalf("storeSplit: %v", err)
			}
			if _, err := db.ExecContext(ctx, corrupt); err != nil {
				t.Fatalf("corrupt state: %v", err)
			}
			if _, _, _, err := st.loadSplit(ctx); err == nil {
				t.Fatalf("loadSplit accepted inconsistent state %v", stateMap(t, db))
			}
		})
	}
}

// ---- T-010: state storage upgrade ----

func TestStateUpgradeLegacyTable(t *testing.T) {
	_, db := requireDB(t)
	ctx := context.Background()

	// Layout created by earlier versions of the tool.
	if _, err := db.ExecContext(ctx, `CREATE TABLE global_block_migration_state (
			set_key VARCHAR(25) NOT NULL PRIMARY KEY,
			value INT NOT NULL DEFAULT 0
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatalf("create legacy state table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO global_block_migration_state (set_key, value) VALUES (?, 123)`,
		legacyWatermarkKey); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	st := newTestState(t, db)
	assertCurrentStateLayout(t, db)

	// Every key the tool writes, with values beyond the INT range.
	want := map[string]int64{legacyWatermarkKey: 123}
	write := func(key string, value int64) {
		t.Helper()
		if _, err := st.insertMigrationStateStm.ExecContext(ctx, key, value, value); err != nil {
			t.Fatalf("write %s: %v", key, err)
		}
		want[key] = value
	}
	write(splitKKey, 3)
	for _, i := range []int{0, 1, 42, 9999} {
		base := int64(5_000_000_000) + int64(i)*10
		write(rangeLowerKey(i), base)
		write(rangeUpperKey(i), base+5)
		write(rangeWatermarkKey(i), base-1)
	}

	got := stateMap(t, db)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state read back:\n got %v\nwant %v", got, want)
	}
	if _, ok := got["range_9999_watermark"]; !ok {
		t.Fatalf("range_9999_watermark not stored under its exact key: %v", got)
	}
	for _, r := range stateRows(t, db) {
		if _, ok := want[r.Key]; !ok {
			t.Fatalf("unexpected (truncated?) key %q", r.Key)
		}
	}

	// Second upgrade: no error, rows and layout unchanged.
	snapshot := stateRows(t, db)
	layout := stateColumns(t, db)
	newTestState(t, db)
	if got := stateRows(t, db); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("second init changed rows:\n got %v\nwant %v", got, snapshot)
	}
	if got := stateColumns(t, db); got != layout {
		t.Fatalf("second init changed layout: got %+v, want %+v", got, layout)
	}
	if err := st.upgrade(ctx, db); err != nil {
		t.Fatalf("third upgrade: %v", err)
	}
	if got := stateRows(t, db); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("third upgrade changed rows")
	}
}

func TestStateInitFreshTableLayout(t *testing.T) {
	_, db := requireDB(t)
	newTestState(t, db)
	assertCurrentStateLayout(t, db)
	newTestState(t, db)
	assertCurrentStateLayout(t, db)
}
