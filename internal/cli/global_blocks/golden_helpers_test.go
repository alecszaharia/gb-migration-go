package global_blocks

// Golden baseline helpers (T-011, consumed by T-028).
//
// The golden files in testdata/golden/ record what the pre-change sequential
// tool (commit 7c5eea1) writes for a deterministic synthetic fixture. They are
// produced by scripts/capture_baseline.sh, which builds the old tool and runs
// TestCaptureGoldenBaseline against it.
//
// Scenarios (both start from a fresh fixture built by seedGoldenFixture, with
// empty migration state, and run with "-b <goldenBatch>"):
//
//   - "normal" (normal.json): run the tool once without --failed. The golden
//     is the normalised migrated data plus the failed table afterwards.
//
//   - "failed" (failed.json): run the tool once without --failed (same as the
//     normal scenario), then applyGoldenFailedFixes repairs the rules of some
//     previously failing blocks, then run the tool with --failed. The golden
//     records the failed table before the --failed run (FailedBefore) and the
//     normalised migrated data plus failed table after it.
//
// T-028 reproduces the exact same inputs with seedGoldenFixture,
// goldenRunArgs / goldenFailedRunArgs and applyGoldenFailedFixes, normalises
// with captureGolden, and compares with assertGoldenEqual.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	// goldenBatch is the --batch size used by every golden run. It is small
	// enough that the fixture spans many batches, with failing blocks mixed
	// into several of them.
	goldenBatch = 7

	goldenDir = "testdata/golden"

	goldenNormal = "normal"
	goldenFailed = "failed"

	// goldenSchemaPlaceholder replaces the throwaway schema name in stored
	// error messages (MySQL FK errors quote `schema`.`table`).
	goldenSchemaPlaceholder = "<schema>"
)

// goldenRunArgs are the tool arguments (after "-d <dsn>") of a normal golden run.
func goldenRunArgs() []string { return []string{"-b", strconv.Itoa(goldenBatch)} }

// goldenFailedRunArgs are the tool arguments (after "-d <dsn>") of the --failed golden run.
func goldenFailedRunArgs() []string {
	return []string{"-b", strconv.Itoa(goldenBatch), "--failed"}
}

// Golden fixture IDs. Fixed and sparse so ID-range based code paths see gaps.
var (
	// plain eligible blocks; rules rotate over fxRuleVariants by id%6, and
	// this list covers every residue.
	goldenEligibleIDs = []int64{
		3, 4, 5, 8, 13, 14, 15, 21, 22, 34, 35, 36, 37, 55, 56, 89, 90, 91,
		144, 145, 146, 147, 233, 234, 377, 378, 379, 610, 611, 612,
		987, 988, 989, 990, 1597, 1598, 2584, 4181, 4182, 6765,
		10946, 10947, 17711, 28657, 46368,
	}

	goldenMalformedIDs      = []int64{7, 100, 5000}
	goldenInvalidIRIIDs     = []int64{12, 400, 20000}
	goldenMissingCTIDs      = []int64{30, 999, 30000}
	goldenWrongNodeIDs      = []int64{6, 250, 12000}
	goldenLegacyAPIIDs      = []int64{9, 700, 25000}
	goldenNoAPIVersionIDs   = []int64{11, 1500, 40000}
	goldenFixedFailingRules = map[int64]string{
		100: rulesSpecificItem,       // was failMalformedRules
		400: rulesExcludeEcwid,       // was failInvalidIRI
		999: rulesCollectionTypeOnly, // was failMissingCollectionType
	}
)

// seedGoldenFixture seeds the deterministic golden fixture: 51 eligible
// blocks that migrate (every rules variant, custom titles, draft status,
// tags, missing rules row, missing compiled data, non-ASCII title), 9
// failing blocks (3 of each failKind) and 9 ineligible rows (wrong node,
// api_version=1, no api_version), all interleaved with gapped IDs.
//
// Both the baseline capture (T-011) and the equivalence test (T-028) must use
// this function unchanged; editing it invalidates testdata/golden.
func seedGoldenFixture(fx *fixture) {
	fx.t.Helper()

	fx.addBlock(blockSpec{ID: 1, Class: classEligible, Title: "Header", Tags: `["header","main"]`})
	fx.addBlock(blockSpec{ID: 2, Class: classEligible, Title: "Draft footer", Status: "draft"})
	fx.addEligible(goldenEligibleIDs...)
	fx.addBlock(blockSpec{ID: 50, Class: classEligible, Title: "No rules row", OmitRules: true})
	fx.addBlock(blockSpec{ID: 60, Class: classEligible, Title: "No compiled data", NoCompiledData: true})
	fx.addBlock(blockSpec{ID: 70, Class: classEligible, Title: "Überschrift – ñ ✓", Rules: rulesMulti, Tags: `["i18n"]`})
	fx.addBlock(blockSpec{ID: 75000, Class: classEligible, Title: "Highest id", Rules: rulesEverywhere})

	fx.addFailing(failMalformedRules, goldenMalformedIDs...)
	fx.addFailing(failInvalidIRI, goldenInvalidIRIIDs...)
	fx.addFailing(failMissingCollectionType, goldenMissingCTIDs...)

	fx.addWrongNode(goldenWrongNodeIDs...)
	fx.addLegacyAPI(goldenLegacyAPIIDs...)
	fx.addNoAPIVersion(goldenNoAPIVersionIDs...)
}

// applyGoldenFailedFixes repairs the migrated_rules of one failing block of
// each failKind (IDs in goldenFixedFailingRules) so that a --failed run can
// migrate them; the other failing blocks keep failing. It also updates the
// fixture's bookkeeping so fx.okIDs()/fx.failingIDs() reflect the repair.
func applyGoldenFailedFixes(fx *fixture) {
	fx.t.Helper()
	for id, rules := range goldenFixedFailingRules {
		res := fx.exec(`UPDATE metafield__text SET value = ? WHERE metafield_id = ? AND entity_id = ?`,
			rules, fxMfMigratedRules, id)
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			fx.t.Fatalf("fix golden block %d: rows affected %d, err %v", id, n, err)
		}
		b := fx.block(id)
		b.Class = classEligible
		b.Rules = rules
		b.RuleCount = fxRuleCount[rules]
		fx.blocks[id] = b
	}
}

// goldenSnapshot is the normalised, comparable output of a tool run:
// migrated data keyed by source data ID (no auto-increment IDs or insert-time
// columns) and the failed table without failed_at, with the throwaway schema
// name replaced by goldenSchemaPlaceholder. The state table is deliberately
// excluded: its layout changes with the parallel tool by design.
type goldenSnapshot struct {
	Migrated []migratedBlock `json:"migrated"`
	Failed   []failedRow     `json:"failed"`
}

// goldenData is the content of a testdata/golden/<name>.json file.
type goldenData struct {
	Scenario    string   `json:"scenario"`
	Description string   `json:"description"`
	BaseCommit  string   `json:"base_commit"`
	Args        []string `json:"args"` // tool args (after -d) of the recorded run
	// FailedBefore is the normalised failed table right before the recorded
	// run (failed scenario only).
	FailedBefore []failedRow `json:"failed_before,omitempty"`
	goldenSnapshot
}

// captureGolden returns the normalised golden snapshot of db.
func captureGolden(t testing.TB, db *sql.DB) goldenSnapshot {
	t.Helper()
	return goldenSnapshot{
		Migrated: migratedBlocks(t, db),
		Failed:   normaliseFailedRows(t, db, failedRows(t, db)),
	}
}

// normaliseFailedRows replaces the connected schema's name in error messages
// with goldenSchemaPlaceholder so runs in different throwaway schemas compare
// equal. The result is never nil.
func normaliseFailedRows(t testing.TB, db *sql.DB, rows []failedRow) []failedRow {
	t.Helper()
	var schema string
	if err := db.QueryRowContext(context.Background(), "SELECT DATABASE()").Scan(&schema); err != nil {
		t.Fatalf("select database: %v", err)
	}
	out := make([]failedRow, 0, len(rows))
	for _, r := range rows {
		if schema != "" {
			r.Error = strings.ReplaceAll(r.Error, schema, goldenSchemaPlaceholder)
		}
		out = append(out, r)
	}
	return out
}

func goldenPath(name string) string { return filepath.Join(goldenDir, name+".json") }

// loadGolden reads testdata/golden/<name>.json.
func loadGolden(t testing.TB, name string) goldenData {
	t.Helper()
	b, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with scripts/capture_baseline.sh)", name, err)
	}
	var g goldenData
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("decode golden %s: %v", name, err)
	}
	return g
}

// writeGolden writes g to testdata/golden/<name>.json.
func writeGolden(t testing.TB, name string, g goldenData) {
	t.Helper()
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatalf("encode golden %s: %v", name, err)
	}
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", goldenDir, err)
	}
	if err := os.WriteFile(goldenPath(name), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", name, err)
	}
}

// assertGoldenEqual fails the test with a per-block diff when got differs
// from want.
func assertGoldenEqual(t testing.TB, name string, want, got goldenSnapshot) {
	t.Helper()
	if d := diffGolden(want, got); d != "" {
		t.Errorf("golden %s mismatch:\n%s", name, d)
	}
}

// diffGolden describes the differences between two golden snapshots ("" when equal).
func diffGolden(want, got goldenSnapshot) string {
	var sb strings.Builder

	wantIDs := make([]int64, 0, len(want.Migrated))
	for _, b := range want.Migrated {
		wantIDs = append(wantIDs, b.DataID)
	}
	gotIDs := make([]int64, 0, len(got.Migrated))
	for _, b := range got.Migrated {
		gotIDs = append(gotIDs, b.DataID)
	}
	if d := diffIDs(gotIDs, wantIDs); d != "" {
		fmt.Fprintf(&sb, "migrated data IDs: %s\n", d)
	} else {
		for i := range want.Migrated {
			if !reflect.DeepEqual(want.Migrated[i], got.Migrated[i]) {
				fmt.Fprintf(&sb, "migrated block %d:\n  want %+v\n  got  %+v\n", want.Migrated[i].DataID, want.Migrated[i], got.Migrated[i])
			}
		}
	}

	wantF, gotF := want.Failed, got.Failed
	if len(wantF) == 0 && len(gotF) == 0 {
		return sb.String()
	}
	if !reflect.DeepEqual(wantF, gotF) {
		fmt.Fprintf(&sb, "failed table:\n  want %+v\n  got  %+v\n", wantF, gotF)
	}
	return sb.String()
}
