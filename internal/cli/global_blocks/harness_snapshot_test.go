package global_blocks

// Snapshot helpers for integration tests. All results are deterministically
// ordered so they can be compared with reflect.DeepEqual / slices.Equal.

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// nullMarker represents SQL NULL in raw row dumps.
const nullMarker = "<NULL>"

// queryRows runs query and returns every row as strings (NULL -> nullMarker).
func queryRows(t testing.TB, db *sql.DB, query string, args ...any) [][]string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("snapshot query %q: %v", firstLine(query), err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("snapshot columns: %v", err)
	}
	var out [][]string
	for rows.Next() {
		raw := make([]sql.RawBytes, len(cols))
		dest := make([]any, len(cols))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatalf("snapshot scan: %v", err)
		}
		row := make([]string, len(cols))
		for i, b := range raw {
			if b == nil {
				row[i] = nullMarker
			} else {
				row[i] = string(b)
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("snapshot rows: %v", err)
	}
	return out
}

// tableExists reports whether table exists in the connected schema.
func tableExists(t testing.TB, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&n)
	if err != nil {
		t.Fatalf("table exists %s: %v", table, err)
	}
	return n > 0
}

// dumpTable returns every row of table ordered by its primary key column(s),
// all columns included. An absent table yields nil.
func dumpTable(t testing.TB, db *sql.DB, table, orderBy string) [][]string {
	t.Helper()
	if !tableExists(t, db, table) {
		return nil
	}
	return queryRows(t, db, fmt.Sprintf("SELECT * FROM `%s` ORDER BY %s", table, orderBy))
}

// migratedTables lists what migrateGlobalBlockWithRules writes.
var migratedTables = []string{"compiled_data", "page_data", "global_block", "rules"}

// dbSnapshot is a raw, byte-exact picture of everything the tool writes:
// migrated tables (incl. auto-increment IDs and timestamps), the failed table
// (without failed_at) and the state table. Use it for "nothing changed" and
// "rows kept their insert IDs" assertions.
type dbSnapshot struct {
	Migrated map[string][][]string
	Failed   []failedRow
	State    []stateRow
}

func takeSnapshot(t testing.TB, db *sql.DB) dbSnapshot {
	t.Helper()
	s := dbSnapshot{Migrated: map[string][][]string{}}
	for _, table := range migratedTables {
		s.Migrated[table] = dumpTable(t, db, table, "id")
	}
	s.Failed = failedRows(t, db)
	s.State = stateRows(t, db)
	return s
}

// failedRow is a global_block_migration_failed row without failed_at.
type failedRow struct {
	DataID int64
	Error  string
}

// failedRows returns the failed table ordered by data_id (nil if absent).
func failedRows(t testing.TB, db *sql.DB) []failedRow {
	t.Helper()
	if !tableExists(t, db, "global_block_migration_failed") {
		return nil
	}
	var out []failedRow
	for _, r := range queryRows(t, db, `SELECT data_id, IFNULL(error,'`+nullMarker+`') FROM global_block_migration_failed ORDER BY data_id`) {
		out = append(out, failedRow{DataID: mustInt(t, r[0]), Error: r[1]})
	}
	return out
}

// failedIDs returns the failed table's data_ids, ascending.
func failedIDs(t testing.TB, db *sql.DB) []int64 {
	t.Helper()
	var out []int64
	for _, r := range failedRows(t, db) {
		out = append(out, r.DataID)
	}
	return out
}

// stateRow is a global_block_migration_state row.
type stateRow struct {
	Key   string
	Value int64
}

// stateRows returns the state table ordered by set_key (nil if absent).
func stateRows(t testing.TB, db *sql.DB) []stateRow {
	t.Helper()
	if !tableExists(t, db, "global_block_migration_state") {
		return nil
	}
	var out []stateRow
	for _, r := range queryRows(t, db, `SELECT set_key, value FROM global_block_migration_state ORDER BY set_key`) {
		out = append(out, stateRow{Key: r[0], Value: mustInt(t, r[1])})
	}
	return out
}

// stateMap returns the state table as key -> value.
func stateMap(t testing.TB, db *sql.DB) map[string]int64 {
	t.Helper()
	m := map[string]int64{}
	for _, r := range stateRows(t, db) {
		m[r.Key] = r.Value
	}
	return m
}

// migratedRule is a rules row without its id and timestamps.
type migratedRule struct {
	ProjectID           string
	Mode                string
	Type                string
	CollectionItem      string
	CollectionType      string
	Customer            string
	CustomerGroup       string
	CollectionTypeSlug  string
	ExternalType        string
	ExternalID          string
	CollectionTypeField string
	FieldValueItem      string
}

// migratedBlock is a global_block row joined with its page_data, compiled_data
// and rules, keyed by the source data ID. Auto-increment IDs and insert-time
// columns (compiled_data.ttl, rules timestamps) are excluded, so two runs of
// the tool on the same fixture compare equal regardless of insert order.
type migratedBlock struct {
	DataID       int64 // source data.id (matched by uid + project + globalblock node)
	ProjectID    int64
	UID          string
	AuthorID     int64
	Title        string
	Status       string
	Meta         string
	Position     string
	Tags         string
	Dependencies string
	CreatedAt    string
	UpdatedAt    string
	PageData     string
	CompiledData string // nullMarker when compiled_data_id is NULL
	Rules        []migratedRule
}

// migratedBlocks returns every migrated global block, ordered by source data
// ID then global_block.id; duplicates (a block migrated twice) appear twice.
// DataID is 0 for a global_block row with no matching source row.
func migratedBlocks(t testing.TB, db *sql.DB) []migratedBlock {
	t.Helper()
	rows := queryRows(t, db, `
		SELECT IFNULL(d.id, 0), gb.id, gb.project_id, gb.uid, gb.author_id, gb.title, gb.status,
		       gb.meta, IFNULL(gb.position,'`+nullMarker+`'), IFNULL(gb.tags,'`+nullMarker+`'),
		       IFNULL(CAST(gb.dependencies AS CHAR),'`+nullMarker+`'),
		       gb.created_at, IFNULL(gb.updated_at,'`+nullMarker+`'),
		       IFNULL(pd.data,'`+nullMarker+`'), IFNULL(cd.data,'`+nullMarker+`')
		FROM global_block gb
		LEFT JOIN data d ON d.uid = gb.uid AND d.parent_id = gb.project_id
		     AND d.node_id = (SELECT id FROM node WHERE slug = 'globalblock')
		LEFT JOIN page_data pd ON pd.id = gb.page_data_id
		LEFT JOIN compiled_data cd ON cd.id = gb.compiled_data_id
		ORDER BY IFNULL(d.id, 0), gb.id`)

	ruleRows := queryRows(t, db, `
		SELECT global_block, project_id, mode, type,
		       IFNULL(collection_item,'`+nullMarker+`'), IFNULL(collection_type,'`+nullMarker+`'),
		       IFNULL(customer,'`+nullMarker+`'), IFNULL(customer_group,'`+nullMarker+`'),
		       IFNULL(collection_type_slug,'`+nullMarker+`'), IFNULL(external_type,'`+nullMarker+`'),
		       IFNULL(external_id,'`+nullMarker+`'), IFNULL(collection_type_field,'`+nullMarker+`'),
		       IFNULL(field_value_item,'`+nullMarker+`')
		FROM rules ORDER BY global_block, id`)
	rulesByGB := map[string][]migratedRule{}
	for _, r := range ruleRows {
		rulesByGB[r[0]] = append(rulesByGB[r[0]], migratedRule{
			ProjectID: r[1], Mode: r[2], Type: r[3], CollectionItem: r[4], CollectionType: r[5],
			Customer: r[6], CustomerGroup: r[7], CollectionTypeSlug: r[8], ExternalType: r[9],
			ExternalID: r[10], CollectionTypeField: r[11], FieldValueItem: r[12],
		})
	}

	out := make([]migratedBlock, 0, len(rows))
	for _, r := range rows {
		out = append(out, migratedBlock{
			DataID: mustInt(t, r[0]), ProjectID: mustInt(t, r[2]), UID: r[3], AuthorID: mustInt(t, r[4]),
			Title: r[5], Status: r[6], Meta: r[7], Position: r[8], Tags: r[9], Dependencies: r[10],
			CreatedAt: r[11], UpdatedAt: r[12], PageData: r[13], CompiledData: r[14],
			Rules: rulesByGB[r[1]],
		})
	}
	return out
}

// migratedDataIDs returns the source data IDs that have a global_block row,
// ascending, with duplicates preserved.
func migratedDataIDs(t testing.TB, db *sql.DB) []int64 {
	t.Helper()
	var out []int64
	for _, b := range migratedBlocks(t, db) {
		out = append(out, b.DataID)
	}
	return out
}

// migratedCountByDataID counts global_block rows per source data ID; any
// value > 1 is a duplicate migration.
func migratedCountByDataID(t testing.TB, db *sql.DB) map[int64]int {
	t.Helper()
	m := map[int64]int{}
	for _, id := range migratedDataIDs(t, db) {
		m[id]++
	}
	return m
}

// rowCounts returns COUNT(*) of each migrated table.
func rowCounts(t testing.TB, db *sql.DB) map[string]int64 {
	t.Helper()
	m := map[string]int64{}
	for _, table := range migratedTables {
		r := queryRows(t, db, "SELECT COUNT(*) FROM `"+table+"`")
		m[table] = mustInt(t, r[0][0])
	}
	return m
}

func mustInt(t testing.TB, s string) int64 {
	t.Helper()
	var v int64
	if _, err := fmt.Sscan(s, &v); err != nil {
		t.Fatalf("parse int %q: %v", s, err)
	}
	return v
}

// diffIDs describes the difference between two ascending ID lists ("" when equal).
func diffIDs(got, want []int64) string {
	if slices.Equal(got, want) {
		return ""
	}
	gs, ws := map[int64]int{}, map[int64]int{}
	for _, id := range got {
		gs[id]++
	}
	for _, id := range want {
		ws[id]++
	}
	var extra, missing []string
	for id, n := range gs {
		if n > ws[id] {
			extra = append(extra, fmt.Sprintf("%d(x%d)", id, n-ws[id]))
		}
	}
	for id, n := range ws {
		if n > gs[id] {
			missing = append(missing, fmt.Sprintf("%d(x%d)", id, n-gs[id]))
		}
	}
	slices.Sort(extra)
	slices.Sort(missing)
	return fmt.Sprintf("unexpected=[%s] missing=[%s]", strings.Join(extra, " "), strings.Join(missing, " "))
}
