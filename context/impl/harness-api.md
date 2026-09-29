# Integration Harness API (T-005)

Build site: context/plans/build-site.md

Package `global_blocks` internal tests. Run with `GB_MIGRATION_TEST_DSN='root:nopassword@tcp(127.0.0.1:3306)/'`; unset → DB tests skip.
Process-global state (viper, captureStdout, setTestHook): DB/command tests must NOT use t.Parallel.

## Hooks (hooks.go, nil in production)
- `var testHookBatchStart, testHookBeforeBatchCommit func(rangeIdx int) error`; `var testHookSplitStoreStep func(i int) error`
- `callHookBatchStart(idx)`, `callHookBeforeBatchCommit(idx)`, `callHookSplitStoreStep(i)` → nil when hook nil. Production code calls these.
- `setTestHook(t, &testHookX, fn)` — sets + restores in t.Cleanup.

## DB / fixture
- `requireDB(t) (dsn string, db *sql.DB)` — throwaway `gbtest_<hex>` schema from embedded testdata/schema.sql; cleanup kills conns + drops. `GB_MIGRATION_TEST_KEEP=1` keeps schema.
- `newFixture(t) *fixture` — fields `fx.DSN`, `fx.DB`; base rows (nodes, metafields, projects). Seed block IDs must be 1..999_999.
- Seeds (return []int64; 0/none = auto ascending): `fx.addEligible(ids...)`, `fx.addEligibleN(n)`, `fx.addFailing(kind, ids...)` (kinds failMalformedRules, failInvalidIRI, failMissingCollectionType; `kind.errorSubstring()`), `fx.addWrongNode`, `fx.addLegacyAPI`, `fx.addNoAPIVersion`, `fx.addBlock(blockSpec{...}) int64`.
- Queries: `fx.block(id)`, `fx.eligibleIDs()` (ok+failing), `fx.okIDs()`, `fx.failingIDs()`, `fx.ineligibleIDs()`, `fx.maxSeededID()`.

## Snapshots
- `takeSnapshot(t, db) dbSnapshot{Migrated, Failed, State}` — raw rows incl. IDs; compare with reflect.DeepEqual.
- `migratedBlocks(t, db) []migratedBlock` — normalised (no auto IDs/timestamps), keyed to source data.id; duplicates kept.
- `migratedDataIDs`, `migratedCountByDataID`, `failedRows` / `failedIDs`, `stateRows` / `stateMap`, `rowCounts`, `dumpTable`, `queryRows`, `tableExists`, `diffIDs(got, want)`.

## Run / stdout
- `captureStdout(t, fn) string`; `runCommand(ctx, args...) error` (viper.Reset + NewCommand + ExecuteContext); `runCommandCapture(t, ctx, args...)`; `fx.run(ctx, args...) (stdout, err)` prepends `-d fx.DSN`.
- Each command run opens its own pool (never closed); requireDB cleanup kills its connections.
