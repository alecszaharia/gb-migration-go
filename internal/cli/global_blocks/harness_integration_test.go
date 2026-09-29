package global_blocks

// Harness self-tests: prove the throwaway schema, seeds and snapshot helpers
// match what the current tool reads and writes. Skipped without
// GB_MIGRATION_TEST_DSN.

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHarnessRequireDBCreatesIsolatedSchema(t *testing.T) {
	var schema string
	t.Run("inner", func(t *testing.T) {
		dsn, db := requireDB(t)
		if err := db.QueryRow("SELECT DATABASE()").Scan(&schema); err != nil {
			t.Fatalf("select database: %v", err)
		}
		if !strings.HasPrefix(schema, testSchemaPrefix) {
			t.Fatalf("schema %q lacks prefix %q", schema, testSchemaPrefix)
		}
		if !strings.Contains(dsn, "/"+schema) {
			t.Fatalf("dsn %q does not point at %s", dsn, schema)
		}
		for _, table := range []string{"node", "metafield", "metafield__int", "metafield__text", "data", "page_data", "compiled_data", "global_block", "rules"} {
			if !tableExists(t, db, table) {
				t.Errorf("table %s missing after loading schema.sql", table)
			}
		}
		if tableExists(t, db, "global_block_migration_state") || tableExists(t, db, "global_block_migration_failed") {
			t.Error("tool-owned tables must not be pre-created")
		}
	})
	if schema == "" {
		return // skipped
	}

	// after cleanup the schema must be gone
	_, db := requireDB(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, schema).Scan(&n); err != nil {
		t.Fatalf("schemata: %v", err)
	}
	if n != 0 {
		t.Fatalf("schema %s not dropped by cleanup", schema)
	}
}

func TestHarnessSeedsAutoAndExplicitIDs(t *testing.T) {
	fx := newFixture(t)

	a := fx.addEligibleN(3)                // 1,2,3
	b := fx.addEligible(10, 0, 25)         // 10,11,25
	c := fx.addFailing(failMalformedRules) // 26
	d := fx.addWrongNode(0)                // 27
	e := fx.addLegacyAPI(40)               // 40
	f := fx.addEligible()                  // 41

	got := slices.Concat(a, b, c, d, e, f)
	want := []int64{1, 2, 3, 10, 11, 25, 26, 27, 40, 41}
	if !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if fx.maxSeededID() != 41 {
		t.Fatalf("maxSeededID = %d, want 41", fx.maxSeededID())
	}

	// the repository's own eligibility queries must see exactly the eligible IDs
	ctx := context.Background()
	repo, err := newPrepareRepository(ctx, fx.DB)
	if err != nil {
		t.Fatalf("newPrepareRepository: %v", err)
	}
	ids, err := repo.getGlobalBlocksIds(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("getGlobalBlocksIds: %v", err)
	}
	if diff := diffIDs(ids, fx.eligibleIDs()); diff != "" {
		t.Fatalf("getGlobalBlocksIds: %s", diff)
	}
	total, err := repo.getTotalCount(ctx, 0)
	if err != nil {
		t.Fatalf("getTotalCount: %v", err)
	}
	if total != int64(len(fx.eligibleIDs())) {
		t.Fatalf("getTotalCount = %d, want %d", total, len(fx.eligibleIDs()))
	}
	gbs, err := repo.getGlobalBLocks(ctx, ids)
	if err != nil {
		t.Fatalf("getGlobalBLocks: %v", err)
	}
	if len(gbs) != len(ids) {
		t.Fatalf("getGlobalBLocks returned %d, want %d", len(gbs), len(ids))
	}
	for _, gb := range gbs {
		sb := fx.block(gb.id.Int64)
		if gb.rules.String != sb.Rules || gb.projectId.Int64 != sb.ProjectID || !gb.compileddataMetafieldValueId.Valid {
			t.Errorf("block %d read back as rules=%q project=%d compiled=%v", gb.id.Int64, gb.rules.String, gb.projectId.Int64, gb.compileddataMetafieldValueId)
		}
	}
}

// TestHarnessCurrentToolEndToEnd runs the current (sequential) command on a
// mixed fixture and checks the seeds behave as their class says.
func TestHarnessCurrentToolEndToEnd(t *testing.T) {
	fx := newFixture(t)

	// interleave every class, with gaps, across several batches of 3
	fx.addEligibleN(4)
	fx.addWrongNode(0)
	fx.addFailing(failMalformedRules)
	fx.addEligible(20, 21)
	fx.addLegacyAPI(0)
	fx.addFailing(failInvalidIRI, 30)
	fx.addNoAPIVersion(0)
	fx.addEligible(50)
	fx.addFailing(failMissingCollectionType)
	fx.addBlock(blockSpec{Class: classEligible, Title: "Header", Status: "draft", Tags: "a,b", OmitRules: true})
	fx.addBlock(blockSpec{Class: classEligible, NoCompiledData: true, Rules: rulesMulti})
	fx.addEligibleN(3)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	before := takeSnapshot(t, fx.DB)
	out, err := fx.run(ctx, "-b", "3")
	if err != nil {
		t.Fatalf("command failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Starting migration") {
		t.Errorf("output lacks start banner:\n%s", out)
	}
	if reflect.DeepEqual(before, takeSnapshot(t, fx.DB)) {
		t.Fatal("snapshot unchanged after migration")
	}

	// eligible OK blocks: migrated exactly once, content matches the source
	blocks := migratedBlocks(t, fx.DB)
	var got []int64
	for _, mb := range blocks {
		got = append(got, mb.DataID)
		sb := fx.block(mb.DataID)
		if mb.ProjectID != sb.ProjectID || mb.UID != sb.UID || mb.AuthorID != fxAuthorID ||
			mb.Title != sb.Title || mb.Status != sb.Status || mb.PageData != sb.Body {
			t.Errorf("block %d migrated as %+v, seeded %+v", mb.DataID, mb, sb)
		}
		if sb.RuleCount >= 0 && len(mb.Rules) != sb.RuleCount {
			t.Errorf("block %d has %d rules, want %d (rules %s)", mb.DataID, len(mb.Rules), sb.RuleCount, sb.Rules)
		}
		if mb.Dependencies != "[]" {
			t.Errorf("block %d dependencies = %q", mb.DataID, mb.Dependencies)
		}
	}
	if diff := diffIDs(got, fx.okIDs()); diff != "" {
		t.Fatalf("migrated IDs: %s", diff)
	}

	// variant-specific checks
	for _, mb := range blocks {
		sb := fx.block(mb.DataID)
		switch {
		case sb.Title == "Header":
			if mb.Status != "draft" || mb.Tags != "a,b" || len(mb.Rules) != 0 {
				t.Errorf("titled draft block migrated as %+v", mb)
			}
		case sb.Rules == rulesMulti && mb.CompiledData == nullMarker:
			if len(mb.Rules) != 2 || mb.Rules[0].Mode != MODE_INCLUDE || mb.Rules[1].Mode != MODE_EXCLUDE {
				t.Errorf("multi-rule block rules = %+v", mb.Rules)
			}
		case sb.Rules == rulesSpecificItem:
			r := mb.Rules[0]
			if r.Type != RULE_TYPE_SPECIFIC || r.CollectionType != "26" || r.CollectionItem != "33" {
				t.Errorf("specific-item rule = %+v", r)
			}
		case sb.Rules == rulesExcludeEcwid:
			r := mb.Rules[0]
			if r.Mode != MODE_EXCLUDE || r.ExternalType != "ecwid-product" || r.ExternalID != "12345" {
				t.Errorf("ecwid rule = %+v", r)
			}
		}
		if mb.CompiledData == nullMarker && sb.Rules != rulesMulti {
			t.Errorf("block %d lost its compiled data", mb.DataID)
		}
	}

	// failing blocks: in the failed table with the expected error, no migrated rows
	failed := failedRows(t, fx.DB)
	var fids []int64
	for _, fr := range failed {
		fids = append(fids, fr.DataID)
		sb := fx.block(fr.DataID)
		if !strings.Contains(fr.Error, sb.Fail.errorSubstring()) {
			t.Errorf("failed block %d error %q lacks %q", fr.DataID, fr.Error, sb.Fail.errorSubstring())
		}
	}
	if diff := diffIDs(fids, fx.failingIDs()); diff != "" {
		t.Fatalf("failed IDs: %s", diff)
	}

	// no orphan rows from rolled-back savepoints
	counts := rowCounts(t, fx.DB)
	wantRules := 0
	for _, id := range fx.okIDs() {
		wantRules += fx.block(id).RuleCount
	}
	if counts["global_block"] != int64(len(fx.okIDs())) || counts["page_data"] != int64(len(fx.okIDs())) ||
		counts["compiled_data"] != int64(len(fx.okIDs())-1) || counts["rules"] != int64(wantRules) {
		t.Fatalf("row counts = %v (ok=%d, rules=%d)", counts, len(fx.okIDs()), wantRules)
	}

	// ineligible rows untouched: neither migrated nor failed
	for _, id := range fx.ineligibleIDs() {
		if slices.Contains(got, id) || slices.Contains(fids, id) {
			t.Errorf("ineligible block %d was processed", id)
		}
	}

	// per-range watermarks: every range ends at its last eligible ID
	eligible := fx.eligibleIDs()
	st := stateMap(t, fx.DB)
	if _, ok := st[legacyWatermarkKey]; ok {
		t.Errorf("state has legacy key %s: %v", legacyWatermarkKey, st)
	}
	k := int(st[splitKKey])
	if k < 1 {
		t.Fatalf("state = %v, want a stored split", st)
	}
	for i := 0; i < k; i++ {
		lower, upper, hasUpper := st[rangeLowerKey(i)], st[rangeUpperKey(i)], i < k-1
		var last int64
		for _, id := range eligible {
			if id >= lower && (!hasUpper || id <= upper) {
				last = id
			}
		}
		if st[rangeWatermarkKey(i)] != last {
			t.Errorf("range %d watermark = %d, want %d (state %v)", i, st[rangeWatermarkKey(i)], last, st)
		}
	}

	// rerun: nothing left, nothing changes
	snap := takeSnapshot(t, fx.DB)
	out, err = fx.run(ctx, "-b", "3")
	if err != nil {
		t.Fatalf("rerun failed: %v", err)
	}
	if !strings.Contains(out, "Nothing to migrate") {
		t.Errorf("rerun output lacks 'Nothing to migrate':\n%s", out)
	}
	if !reflect.DeepEqual(snap, takeSnapshot(t, fx.DB)) {
		t.Fatal("rerun changed the database")
	}

	// --failed: still-failing blocks are re-recorded, migrated data unchanged
	out, err = fx.run(ctx, "-b", "3", "--failed")
	if err != nil {
		t.Fatalf("--failed run failed: %v\n%s", err, out)
	}
	after := takeSnapshot(t, fx.DB)
	if !reflect.DeepEqual(snap.Migrated, after.Migrated) || !reflect.DeepEqual(snap.State, after.State) {
		t.Fatal("--failed run changed migrated data or state")
	}
	if !reflect.DeepEqual(snap.Failed, after.Failed) {
		t.Fatalf("--failed run changed failed rows: %v -> %v", snap.Failed, after.Failed)
	}
}

// TestHarnessCancelledContextRollsBack shows a cancelled context aborts the
// command with an error and leaves no partially migrated batch behind.
func TestHarnessCancelledContextRollsBack(t *testing.T) {
	fx := newFixture(t)
	fx.addEligibleN(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fx.run(ctx, "-b", "2"); err == nil {
		t.Fatal("command succeeded with a cancelled context")
	}
	if n := len(migratedBlocks(t, fx.DB)); n != 0 {
		t.Fatalf("%d blocks migrated with a cancelled context", n)
	}
}
