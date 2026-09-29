package global_blocks

// --failed isolation from workers and split state (T-026, PE R7 c1-c3).
//
// "At most one batch in flight" is measured in the database: AFTER triggers
// on every table the --failed batch writes (global_block inserts, failed
// table inserts and deletes) log the writing connection and its InnoDB
// transaction. A batch is exactly one transaction on one connection, so a
// single distinct (connection, transaction) pair across the whole --failed run
// proves no two batches ever ran, let alone concurrently, even with -w 8.
// The range worker hooks double-check that no range worker ran at all.

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
)

// installWriteLog creates gbtest_write_log plus AFTER triggers that record
// (table, CONNECTION_ID(), current InnoDB trx id) for every row written to
// global_block and global_block_migration_failed.
func installWriteLog(fx *fixture) {
	fx.t.Helper()
	fx.exec(`CREATE TABLE gbtest_write_log (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		tbl VARCHAR(64) NOT NULL,
		op VARCHAR(8) NOT NULL,
		conn BIGINT UNSIGNED NOT NULL,
		trx VARCHAR(64) NULL
	) ENGINE=InnoDB`)
	for _, tr := range []struct{ name, event, table string }{
		{"gbtest_log_gb_ins", "INSERT", "global_block"},
		{"gbtest_log_failed_ins", "INSERT", "global_block_migration_failed"},
		{"gbtest_log_failed_del", "DELETE", "global_block_migration_failed"},
	} {
		fx.exec(fmt.Sprintf(`CREATE TRIGGER %s AFTER %s ON %s FOR EACH ROW
			INSERT INTO gbtest_write_log (tbl, op, conn, trx) VALUES ('%s', '%s', CONNECTION_ID(),
				(SELECT trx_id FROM information_schema.INNODB_TRX WHERE trx_mysql_thread_id = CONNECTION_ID()))`,
			tr.name, tr.event, tr.table, tr.table, tr.event))
	}
}

// writeLogTransactions returns the number of logged writes and the distinct
// (connection, transaction) pairs that made them.
func writeLogTransactions(t *testing.T, fx *fixture) (writes int, txs []string) {
	t.Helper()
	for _, r := range queryRows(t, fx.DB, `SELECT conn, COALESCE(trx, 'NULL') FROM gbtest_write_log ORDER BY id`) {
		writes++
		if key := r[0] + "/" + r[1]; !slices.Contains(txs, key) {
			txs = append(txs, key)
		}
	}
	return writes, txs
}

// forbidRangeWorkers fails the test if any range worker batch starts or
// commits (the --failed path must never reach runRange).
func forbidRangeWorkers(t *testing.T) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	count := func(int) error { calls.Add(1); return nil }
	setTestHook(t, &testHookBatchStart, count)
	setTestHook(t, &testHookBeforeBatchCommit, count)
	return &calls
}

// TestFailedModeIgnoresStoredSplitAndWorkers: a stored K=4 split with
// watermarks and remaining work in the last range, failed rows (three of them
// repaired), then --failed -w 8.
func TestFailedModeIgnoresStoredSplitAndWorkers(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	seedGoldenFixture(fx)

	if out, err := fx.run(ctx, append(goldenRunArgs(), "-w", "4")...); err != nil {
		t.Fatalf("normal run failed: %v\n%s", err, out)
	}
	st := stateMap(t, fx.DB)
	if st[splitKKey] != 4 {
		t.Fatalf("split_k = %d, want 4 (state %v)", st[splitKKey], st)
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Fatalf("failed rows before --failed: %s", d)
	}

	// remaining work: new eligible blocks above the split maximum belong to
	// the open-ended last range, past its watermark
	pending := fx.addEligible(80000, 80001)
	applyGoldenFailedFixes(fx)
	installWriteLog(fx)
	calls := forbidRangeWorkers(t)

	stateBefore := stateRows(t, fx.DB)
	migratedBefore := migratedDataIDs(t, fx.DB)

	out, err := fx.run(ctx, append(goldenFailedRunArgs(), "-w", "8")...)
	if err != nil {
		t.Fatalf("--failed -w 8 run failed: %v\n%s", err, out)
	}

	// PE R7 c1/c2: the split and every watermark are untouched
	if after := stateRows(t, fx.DB); !reflect.DeepEqual(stateBefore, after) {
		t.Errorf("--failed changed the state table:\nbefore %v\nafter  %v", stateBefore, after)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("range worker hooks fired %d times during --failed", n)
	}

	// PE R7 c3: one batch, one transaction, on one connection
	writes, txs := writeLogTransactions(t, fx)
	if writes == 0 {
		t.Fatal("write log is empty: --failed wrote nothing")
	}
	if len(txs) != 1 {
		t.Errorf("--failed writes came from %d (connection/transaction) pairs, want 1: %v", len(txs), txs)
	}

	// only the repaired blocks were migrated; the range work stays pending
	migrated := migratedDataIDs(t, fx.DB)
	var fixed []int64
	for id := range goldenFixedFailingRules {
		fixed = append(fixed, id)
	}
	wantMigrated := append(slices.Clone(migratedBefore), fixed...)
	slices.Sort(wantMigrated)
	if d := diffIDs(migrated, wantMigrated); d != "" {
		t.Errorf("migrated IDs after --failed: %s", d)
	}
	for _, id := range pending {
		if slices.Contains(migrated, id) {
			t.Errorf("pending range block %d was migrated by --failed", id)
		}
	}
	if d := diffIDs(failedIDs(t, fx.DB), fx.failingIDs()); d != "" {
		t.Errorf("failed rows after --failed: %s", d)
	}
}

// TestFailedModeWithoutSplit: --failed on a database that never had a split
// (fresh tables, then failed rows recorded directly) never creates one.
func TestFailedModeWithoutSplit(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	ok := fx.addEligible(10, 20, 30)
	fx.addFailing(failMalformedRules, 15)
	fx.addFailing(failInvalidIRI, 25)
	fx.addEligible(40, 50) // never in the failed table: must stay unmigrated
	calls := forbidRangeWorkers(t)

	// first run creates the state tables; the failed table is empty
	if out, err := fx.run(ctx, "-b", "2", "-w", "8", "--failed"); err != nil {
		t.Fatalf("--failed on empty state failed: %v\n%s", err, out)
	}
	if keys := splitKeys(t, fx.DB); len(keys) != 0 {
		t.Fatalf("--failed on empty state stored split keys %v", keys)
	}
	if ids := migratedDataIDs(t, fx.DB); len(ids) != 0 {
		t.Fatalf("--failed on empty state migrated %v", ids)
	}

	// record failed rows as an earlier (pre-split) run would have
	failedSeed := append(slices.Clone(ok), 15, 25)
	for _, id := range failedSeed {
		fx.exec(`INSERT INTO global_block_migration_failed (data_id, error) VALUES (?, 'earlier failure')`, id)
	}
	installWriteLog(fx)
	stateBefore := stateRows(t, fx.DB)

	if out, err := fx.run(ctx, "-b", "2", "-w", "8", "--failed"); err != nil {
		t.Fatalf("--failed run failed: %v\n%s", err, out)
	}

	if keys := splitKeys(t, fx.DB); len(keys) != 0 {
		t.Errorf("--failed stored split keys %v", keys)
	}
	if after := stateRows(t, fx.DB); !reflect.DeepEqual(stateBefore, after) {
		t.Errorf("--failed changed the state table:\nbefore %v\nafter  %v", stateBefore, after)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("range worker hooks fired %d times during --failed", n)
	}
	if _, txs := writeLogTransactions(t, fx); len(txs) != 1 {
		t.Errorf("--failed writes came from %d (connection/transaction) pairs, want 1: %v", len(txs), txs)
	}

	if d := diffIDs(migratedDataIDs(t, fx.DB), ok); d != "" {
		t.Errorf("migrated IDs: %s", d)
	}
	if d := diffIDs(failedIDs(t, fx.DB), []int64{15, 25}); d != "" {
		t.Errorf("failed IDs: %s", d)
	}
}
