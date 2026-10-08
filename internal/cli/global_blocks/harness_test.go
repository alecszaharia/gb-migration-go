package global_blocks

// MySQL integration test harness.
//
// Integration tests call requireDB (raw throwaway schema) or newFixture
// (throwaway schema + base rows + seed builders). Both skip the test when
// GB_MIGRATION_TEST_DSN is unset, e.g.
//
//	GB_MIGRATION_TEST_DSN='root:nopassword@tcp(127.0.0.1:3306)/' go test ./...
//
// Every test gets its own schema named gbtest_<random>, loaded from
// testdata/schema.sql and dropped in t.Cleanup. Set GB_MIGRATION_TEST_KEEP=1
// to keep the schema for debugging (its name is logged).

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	testDSNEnv       = "GB_MIGRATION_TEST_DSN"
	testKeepEnv      = "GB_MIGRATION_TEST_KEEP"
	testSchemaPrefix = "gbtest_"
)

//go:embed testdata/schema.sql
var testSchemaDDL string

// requireDB skips the test when GB_MIGRATION_TEST_DSN is unset. Otherwise it
// creates a throwaway schema loaded with testdata/schema.sql, registers its
// removal with t.Cleanup, and returns a DSN pointing at that schema (usable as
// the command's --database_url) and an open *sql.DB on it.
//
// The *sql.DB is configured like database.NewDB (interpolated params,
// multi statements) with a pool large enough for parallel-worker tests.
func requireDB(t testing.TB) (dsn string, db *sql.DB) {
	t.Helper()

	base := os.Getenv(testDSNEnv)
	if base == "" {
		t.Skipf("%s not set; skipping MySQL integration test", testDSNEnv)
	}

	baseCfg, err := mysql.ParseDSN(base)
	if err != nil {
		t.Fatalf("parse %s: %v", testDSNEnv, err)
	}

	schema := newTestSchemaName(t)

	adminCfg := baseCfg.Clone()
	adminCfg.DBName = ""
	admin := openTestDB(t, adminCfg, 4)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+schema+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}

	// Registered first so it runs last: after every *sql.DB on the schema
	// has been closed by later-registered cleanups.
	t.Cleanup(func() {
		defer admin.Close()
		if os.Getenv(testKeepEnv) != "" {
			t.Logf("keeping test schema %s (%s set)", schema, testKeepEnv)
			return
		}
		dropTestSchema(t, admin, schema)
	})

	schemaCfg := baseCfg.Clone()
	schemaCfg.DBName = schema

	db = openTestDB(t, schemaCfg, 64)
	t.Cleanup(func() { _ = db.Close() })

	// schema.sql is a mysqldump: one multi-statement Exec keeps its session
	// SETs (FOREIGN_KEY_CHECKS, SQL_MODE, ...) on a single connection.
	if _, err := db.ExecContext(ctx, testSchemaDDL); err != nil {
		t.Fatalf("load testdata/schema.sql into %s: %v", schema, err)
	}

	cmdCfg := baseCfg.Clone()
	cmdCfg.DBName = schema
	return cmdCfg.FormatDSN(), db
}

// openTestDB opens a pool configured like database.NewDB.
func openTestDB(t testing.TB, cfg *mysql.Config, maxOpen int) *sql.DB {
	t.Helper()

	cfg = cfg.Clone()
	cfg.InterpolateParams = true
	cfg.MultiStatements = true

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatalf("mysql connector: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	db.SetConnMaxLifetime(3 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping test MySQL (%s): %v", cfg.Addr, err)
	}
	return db
}

func newTestSchemaName(t testing.TB) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("random schema name: %v", err)
	}
	return testSchemaPrefix + hex.EncodeToString(b[:])
}

// dropTestSchema kills every connection still using the schema (the command
// under test opens its own pool and never closes it, and fault-injection
// tests may leave transactions open), then drops it.
func dropTestSchema(t testing.TB, admin *sql.DB, schema string) {
	t.Helper()

	if !strings.HasPrefix(schema, testSchemaPrefix) {
		t.Errorf("refusing to drop non-test schema %q", schema)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	rows, err := admin.QueryContext(ctx, `SELECT ID FROM information_schema.PROCESSLIST WHERE DB = ? AND ID <> CONNECTION_ID()`, schema)
	if err != nil {
		t.Errorf("list connections of %s: %v", schema, err)
	} else {
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		_ = rows.Close()
		for _, id := range ids {
			// the connection may already be gone
			_, _ = admin.ExecContext(ctx, fmt.Sprintf("KILL %d", id))
		}
	}

	if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+schema+"`"); err != nil {
		t.Errorf("drop test schema %s: %v", schema, err)
	}
}

// ---------------------------------------------------------------------------
// Fixture: base rows + seed builders
// ---------------------------------------------------------------------------

// Node and metafield ids mirror the brizy-cms database the tool targets.
const (
	fxNodeUser        int64 = 1
	fxNodeContainer   int64 = 2
	fxNodeProject     int64 = 5
	fxNodePage        int64 = 7
	fxNodeGlobalBlock int64 = 10

	fxMfApiVersion    int64 = 35  // project, int
	fxMfMeta          int64 = 21  // globalblock, text
	fxMfVersion       int64 = 137 // globalblock, int
	fxMfRules         int64 = 138 // globalblock, text (legacy, unused by the tool)
	fxMfPosition      int64 = 139 // globalblock, text
	fxMfMigratedRules int64 = 140 // globalblock, text
	fxMfTags          int64 = 141 // globalblock, text
	fxMfDependencies  int64 = 142 // globalblock, text
	fxMfCompiledData  int64 = 143 // globalblock, text

	fxAuthorID int64 = 100

	// Base data rows live at or above fxReservedDataID; seeded blocks
	// (explicit or auto-assigned IDs) must stay below it.
	fxReservedDataID      int64 = 1_000_000
	fxContainerID         int64 = fxReservedDataID
	fxProjectID           int64 = fxReservedDataID + 1 // api_version = 2
	fxLegacyProjectID     int64 = fxReservedDataID + 2 // api_version = 1
	fxNoApiVersionProject int64 = fxReservedDataID + 3 // no api_version row

	fxCollectionTypeID int64 = 26
	fxCollectionItemID int64 = 33
)

// Rules JSON variants accepted by rule_converter.go, as stored in the
// migrated_rules metafield. Each converts to fxRuleCount[json] rules rows.
const (
	// specific item of a collection type: collection_type + collection_item
	rulesSpecificItem = `[{"type":1,"appliedFor":1,"entityType":"/collection_types/26","entityValues":["/collection_items/33"],"mode":"specific"}]`
	// whole collection type: specific collection_type, no mode
	rulesCollectionTypeOnly = `[{"type":1,"appliedFor":1,"entityType":"/collection_types/26","entityValues":[]}]`
	// exclude an ecwid product: external_type/external_id, no FKs
	rulesExcludeEcwid = `[{"type":2,"appliedFor":1,"entityType":"ecwid-product","entityValues":["ecwid-product/12345"]}]`
	// everywhere: no entity type, no values
	rulesEverywhere = `[{"type":1,"appliedFor":null,"entityType":"","entityValues":[]}]`
	// two rules in one block
	rulesMulti = `[{"type":1,"appliedFor":1,"entityType":"/collection_types/26","entityValues":["/collection_items/33"],"mode":"specific"},{"type":2,"appliedFor":1,"entityType":"/collection_types/26","entityValues":[]}]`
	// no rules at all
	rulesNone = `[]`
)

var fxRuleCount = map[string]int{
	rulesSpecificItem:       1,
	rulesCollectionTypeOnly: 1,
	rulesExcludeEcwid:       1,
	rulesEverywhere:         1,
	rulesMulti:              2,
	rulesNone:               0,
}

// fxRuleVariants is the rotation used for blocks seeded without explicit rules.
var fxRuleVariants = []string{rulesSpecificItem, rulesCollectionTypeOnly, rulesExcludeEcwid, rulesEverywhere, rulesMulti, rulesNone}

// failKind selects how a seeded eligible block fails migrateGlobalBlockWithRules.
type failKind int

const (
	// failMalformedRules: migrated_rules is not valid JSON (json.Unmarshal error).
	failMalformedRules failKind = iota
	// failInvalidIRI: rule references an unknown IRI table (rule converter error).
	failInvalidIRI
	// failMissingCollectionType: rule references a collection_type that does
	// not exist, so the rules INSERT fails on its foreign key.
	failMissingCollectionType
)

func (k failKind) rulesJSON() string {
	switch k {
	case failMalformedRules:
		return `[{"type":1,"appliedFor":1,"entityType":"/collection_types/26",`
	case failInvalidIRI:
		return `[{"type":1,"appliedFor":1,"entityType":"/bogus_table/26","entityValues":[]}]`
	case failMissingCollectionType:
		return `[{"type":1,"appliedFor":1,"entityType":"/collection_types/999999","entityValues":[]}]`
	}
	panic(fmt.Sprintf("unknown failKind %d", k))
}

// errorSubstring is a stable fragment of the error recorded in the failed table.
func (k failKind) errorSubstring() string {
	switch k {
	case failMalformedRules:
		return "failed to unmarshal the rules json"
	case failInvalidIRI:
		return "invalid iri"
	case failMissingCollectionType:
		return "failed to insert rule"
	}
	panic(fmt.Sprintf("unknown failKind %d", k))
}

// blockClass says whether and how the tool should treat a seeded row.
type blockClass int

const (
	classEligible     blockClass = iota // migrates successfully
	classFailing                        // eligible, fails migration
	classWrongNode                      // ineligible: node is page, not globalblock
	classLegacyAPI                      // ineligible: project api_version = 1
	classNoAPIVersion                   // ineligible: project has no api_version row
)

// blockSpec describes one seeded data row. Zero values get defaults.
type blockSpec struct {
	ID             int64    // 0: auto-assign the next ascending ID
	ProjectID      int64    // 0: derived from the class
	NodeID         int64    // 0: derived from the class
	UID            string   // "": derived from ID
	Title          string   // "": NULL title (migrates as "Unnamed global block")
	Status         string   // "": "publish"
	Body           string   // "": generated section JSON
	Rules          string   // "": rotating variant from fxRuleVariants
	OmitRules      bool     // no migrated_rules row (query falls back to '[]')
	Tags           string   // "": no tags row
	NoCompiledData bool     // no compiledData row (compiled_data_id NULL)
	Fail           failKind // only used when Class == classFailing
	Class          blockClass
}

// seededBlock is what the fixture remembers about a seeded row.
type seededBlock struct {
	ID        int64
	ProjectID int64
	UID       string
	Title     string // expected migrated title
	Status    string // expected migrated status
	Body      string
	Rules     string // effective migrated_rules JSON
	RuleCount int    // expected rules rows (eligible only)
	Class     blockClass
	Fail      failKind
}

// fixture is a throwaway schema with the base rows the tool needs (nodes,
// metafields, author, projects, a collection type and item) plus seed builders.
type fixture struct {
	t      testing.TB
	DSN    string // command --database_url
	DB     *sql.DB
	nextID int64
	blocks map[int64]seededBlock
}

// newFixture calls requireDB (skipping without a DSN) and seeds the base rows.
func newFixture(t testing.TB) *fixture {
	t.Helper()
	dsn, db := requireDB(t)
	fx := &fixture{t: t, DSN: dsn, DB: db, nextID: 1, blocks: map[int64]seededBlock{}}
	fx.seedBase()
	return fx
}

const fxTimestamp = "2026-09-27 15:16:42"

func (fx *fixture) exec(query string, args ...any) sql.Result {
	fx.t.Helper()
	res, err := fx.DB.ExecContext(context.Background(), query, args...)
	if err != nil {
		fx.t.Fatalf("fixture exec %q: %v", firstLine(query), err)
	}
	return res
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (fx *fixture) seedBase() {
	fx.t.Helper()

	fx.exec(`INSERT INTO node (id,parent_id,name,slug,entity_class,default_role_uid,is_file,created_at,updated_at) VALUES
		(1,NULL,'User','user','AppBundle\\Entity\\User',NULL,0,?,?),
		(2,NULL,'Container','container','AppBundle\\Entity\\Data','admin',0,?,?),
		(5,2,'Project','project','AppBundle\\Entity\\Data',NULL,0,?,?),
		(7,5,'Page','page','AppBundle\\Entity\\Data',NULL,0,?,?),
		(10,5,'GlobalBlock','globalblock','AppBundle\\Entity\\Data',NULL,0,?,?)`,
		repeatArgs(10, fxTimestamp)...)

	fx.exec(`INSERT INTO metafield (id,node_id,name,type,created_at,updated_at) VALUES
		(35,5,'api_version','int',?,?),
		(21,10,'meta','text',?,?),
		(137,10,'version','int',?,?),
		(138,10,'rules','text',?,?),
		(139,10,'position','text',?,?),
		(140,10,'migrated_rules','text',?,?),
		(141,10,'tags','text',?,?),
		(142,10,'dependencies','text',?,?),
		(143,10,'compiledData','text',?,?)`,
		repeatArgs(18, fxTimestamp)...)

	fx.exec(`INSERT INTO user (id,application_id,node_id,user_remote_id,token,outdated,locked,approved,status,created_at,updated_at)
		VALUES (?,NULL,1,1000,'test-token',0,0,1,'approved',?,?)`, fxAuthorID, fxTimestamp, fxTimestamp)

	fx.exec(`INSERT INTO data (id,parent_id,node_id,uid,hash_id,status,title,author_id,version,created_at,updated_at)
		VALUES (?,NULL,2,'fx-container',NULL,NULL,'My Personal Projects',?,1,?,?)`,
		fxContainerID, fxAuthorID, fxTimestamp, fxTimestamp)

	fx.addProject(fxProjectID, 2)
	fx.addProject(fxLegacyProjectID, 1)
	fx.addProject(fxNoApiVersionProject, 0)

	fx.exec(`INSERT INTO collection_type (id,category_id,project_id,created_at,priority,title,editor_id,settings,slug)
		VALUES (?,NULL,?,?,1,'Pages',NULL,NULL,'page')`, fxCollectionTypeID, fxProjectID, fxTimestamp)
	fx.exec(`INSERT INTO collection_item (id,type_id,status,created_at,title,slug,project_id,author_id)
		VALUES (?,?,'published',?,'Home','home',?,?)`, fxCollectionItemID, fxCollectionTypeID, fxTimestamp, fxProjectID, fxAuthorID)
}

// addProject inserts a project data row under the container. apiVersion 0
// means no api_version metafield row at all.
func (fx *fixture) addProject(id int64, apiVersion int) {
	fx.t.Helper()
	fx.exec(`INSERT INTO data (id,parent_id,node_id,uid,status,title,body,author_id,version,created_at,updated_at)
		VALUES (?,?,5,?,'active',?,'{}',?,1,?,?)`,
		id, fxContainerID, fmt.Sprintf("fx-project-%d", id), fmt.Sprintf("Project %d", id), fxAuthorID, fxTimestamp, fxTimestamp)
	if apiVersion != 0 {
		fx.exec(`INSERT INTO metafield__int (metafield_id,value,entity_id,created_at,updated_at) VALUES (?,?,?,?,?)`,
			fxMfApiVersion, apiVersion, id, fxTimestamp, fxTimestamp)
	}
}

func repeatArgs(n int, v any) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// allocID returns id, or the next ascending ID when id == 0, and keeps the
// auto-assign cursor above every seeded ID.
func (fx *fixture) allocID(id int64) int64 {
	fx.t.Helper()
	if id == 0 {
		id = fx.nextID
	}
	if id <= 0 || id >= fxReservedDataID {
		fx.t.Fatalf("seeded block id %d out of range (1..%d)", id, fxReservedDataID-1)
	}
	if _, dup := fx.blocks[id]; dup {
		fx.t.Fatalf("seeded block id %d already used", id)
	}
	if id >= fx.nextID {
		fx.nextID = id + 1
	}
	return id
}

// addBlock seeds one data row (+ metafield__text rows) described by spec and
// returns its data ID.
func (fx *fixture) addBlock(spec blockSpec) int64 {
	fx.t.Helper()

	id := fx.allocID(spec.ID)

	project, node := spec.ProjectID, spec.NodeID
	switch spec.Class {
	case classEligible, classFailing:
		if project == 0 {
			project = fxProjectID
		}
		if node == 0 {
			node = fxNodeGlobalBlock
		}
	case classWrongNode:
		if project == 0 {
			project = fxProjectID
		}
		if node == 0 {
			node = fxNodePage
		}
	case classLegacyAPI:
		if project == 0 {
			project = fxLegacyProjectID
		}
		if node == 0 {
			node = fxNodeGlobalBlock
		}
	case classNoAPIVersion:
		if project == 0 {
			project = fxNoApiVersionProject
		}
		if node == 0 {
			node = fxNodeGlobalBlock
		}
	}

	uid := spec.UID
	if uid == "" {
		uid = fmt.Sprintf("gb%010d", id)
	}
	status := spec.Status
	if status == "" {
		status = "publish"
	}
	body := spec.Body
	if body == "" {
		body = fmt.Sprintf(`{"type":"Section","blockId":"block%d","value":{"_styles":["section"],"items":[],"_id":"%s"}}`, id, uid)
	}

	rules := spec.Rules
	if spec.Class == classFailing {
		rules = spec.Fail.rulesJSON()
	} else if rules == "" && !spec.OmitRules {
		rules = fxRuleVariants[int(id)%len(fxRuleVariants)]
	}

	var title any
	if spec.Title != "" {
		title = spec.Title
	}

	created := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second).Format(time.DateTime)
	updated := time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second).Format(time.DateTime)

	fx.exec(`INSERT INTO data (id,parent_id,node_id,uid,hash_id,lang_code,status,title,slug,body,author_id,version,created_at,updated_at)
		VALUES (?,?,?,?,NULL,'en',?,?,NULL,?,?,1,?,?)`,
		id, project, node, uid, status, title, body, fxAuthorID, created, updated)

	text := func(mf int64, v string) {
		fx.exec(`INSERT INTO metafield__text (metafield_id,value,entity_id,created_at,updated_at) VALUES (?,?,?,?,?)`,
			mf, v, id, created, updated)
	}
	text(fxMfMeta, fmt.Sprintf(`{"extraFontStyles":[],"type":"normal","_thumbnailSrc":%d,"_thumbnailWidth":600,"_thumbnailHeight":316}`, id))
	text(fxMfPosition, fmt.Sprintf(`{"align":"top","top":%d,"bottom":%d}`, id%5, id%5))
	if !spec.OmitRules || spec.Class == classFailing {
		text(fxMfMigratedRules, rules)
	}
	if spec.Tags != "" {
		text(fxMfTags, spec.Tags)
	}
	text(fxMfDependencies, `[]`)
	if !spec.NoCompiledData {
		text(fxMfCompiledData, fmt.Sprintf(`{"blocks":[{"id":"%s","html":"<section id=\"%s\"></section>","assets":[]}]}`, uid, uid))
	}

	effRules := rules
	if spec.OmitRules && spec.Class != classFailing {
		effRules = rulesNone
	}
	expTitle := spec.Title
	if expTitle == "" {
		expTitle = "Unnamed global block"
	}
	expStatus := "published"
	if status == "draft" {
		expStatus = "draft"
	}
	count, ok := fxRuleCount[effRules]
	if !ok && spec.Class == classEligible {
		count = -1 // custom rules: unknown count
	}

	fx.blocks[id] = seededBlock{
		ID: id, ProjectID: project, UID: uid, Title: expTitle, Status: expStatus,
		Body: body, Rules: effRules, RuleCount: count, Class: spec.Class, Fail: spec.Fail,
	}
	return id
}

// addEligible seeds eligible blocks that migrate successfully. With no ids it
// seeds one block with an auto-assigned ID; otherwise one block per given ID
// (0 entries are auto-assigned). Returns the IDs.
func (fx *fixture) addEligible(ids ...int64) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classEligible}, ids)
}

// addEligibleN seeds n eligible blocks with auto-assigned ascending IDs.
func (fx *fixture) addEligibleN(n int) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classEligible}, make([]int64, n))
}

// addFailing seeds eligible blocks that fail migration with the given kind.
func (fx *fixture) addFailing(kind failKind, ids ...int64) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classFailing, Fail: kind}, ids)
}

// addWrongNode seeds ineligible rows under an api_version=2 project whose
// node is page instead of globalblock.
func (fx *fixture) addWrongNode(ids ...int64) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classWrongNode}, ids)
}

// addLegacyAPI seeds ineligible global blocks under an api_version=1 project.
func (fx *fixture) addLegacyAPI(ids ...int64) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classLegacyAPI}, ids)
}

// addNoAPIVersion seeds ineligible global blocks under a project without an
// api_version row.
func (fx *fixture) addNoAPIVersion(ids ...int64) []int64 {
	fx.t.Helper()
	return fx.addClass(blockSpec{Class: classNoAPIVersion}, ids)
}

func (fx *fixture) addClass(spec blockSpec, ids []int64) []int64 {
	fx.t.Helper()
	if len(ids) == 0 {
		ids = []int64{0}
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		s := spec
		s.ID = id
		out = append(out, fx.addBlock(s))
	}
	return out
}

// block returns what was seeded for id.
func (fx *fixture) block(id int64) seededBlock {
	fx.t.Helper()
	b, ok := fx.blocks[id]
	if !ok {
		fx.t.Fatalf("no seeded block %d", id)
	}
	return b
}

func (fx *fixture) idsWhere(keep func(seededBlock) bool) []int64 {
	var out []int64
	for id, b := range fx.blocks {
		if keep(b) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// eligibleIDs: every ID the tool's eligibility predicate selects (OK + failing), ascending.
func (fx *fixture) eligibleIDs() []int64 {
	return fx.idsWhere(func(b seededBlock) bool { return b.Class == classEligible || b.Class == classFailing })
}

// okIDs: eligible IDs expected to migrate successfully, ascending.
func (fx *fixture) okIDs() []int64 {
	return fx.idsWhere(func(b seededBlock) bool { return b.Class == classEligible })
}

// failingIDs: eligible IDs expected to land in the failed table, ascending.
func (fx *fixture) failingIDs() []int64 {
	return fx.idsWhere(func(b seededBlock) bool { return b.Class == classFailing })
}

// ineligibleIDs: seeded rows the tool must never touch, ascending.
func (fx *fixture) ineligibleIDs() []int64 {
	return fx.idsWhere(func(b seededBlock) bool {
		return b.Class == classWrongNode || b.Class == classLegacyAPI || b.Class == classNoAPIVersion
	})
}

// maxSeededID is the highest seeded block ID (0 when none).
func (fx *fixture) maxSeededID() int64 { return fx.nextID - 1 }
