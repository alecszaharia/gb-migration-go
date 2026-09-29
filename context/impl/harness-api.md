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

## Golden (T-011, golden_helpers_test.go)
- `seedGoldenFixture(fx)` — fixed synthetic fixture (51 ok, 9 failing = 3/failKind, 9 ineligible; sparse IDs, max 75000). Editing it invalidates goldens → rerun `GIT=/usr/bin/git GB_MIGRATION_TEST_DSN=... scripts/capture_baseline.sh` (builds 7c5eea1 in temp worktree).
- `goldenBatch=7`; `goldenRunArgs()` → `-b 7`; `goldenFailedRunArgs()` → `-b 7 --failed`. New tool normal run: add `-w 1`.
- `applyGoldenFailedFixes(fx)` — fixes blocks 100, 400, 999 before `--failed` run.
- `captureGolden(t, db) goldenSnapshot{Migrated, Failed}` (normalised; state table excluded; schema name → `<schema>`), `loadGolden(t, goldenNormal|goldenFailed) goldenData`, `assertGoldenEqual(t, name, want, got)`.
- Normal: fresh fixture → seed → run goldenRunArgs → 51 migrated, 9 failed. Failed: fresh → seed → normal run → applyGoldenFailedFixes → run goldenFailedRunArgs → 54 migrated, 6 failed (`failed_before` stored).

## State API (T-009, state.go)
- `storedRange{idRange; Watermark int64}`; `(s *state) loadSplit(ctx) (k, []storedRange, ok, err)`; `(s *state) storeSplit(ctx, db, []idRange) error` (one tx; hook step 0..splitStoreWrites(ranges)); `(s *state) updateRangeWatermark(ctx, idx, id)` (use on `withTx` copy). Test helper name `i64` already taken.

## Migration API (T-013/T-014, migration.go)
- `ensureSplit(ctx, db *sql.DB, migSt *state, repo *repository, requestedK int) ([]storedRange, error)` — nil,nil + prints `Nothing to migrate` when no eligible.
- `runRange(ctx, migSt *state, repo *repository, r storedRange, batch int, p *progress) error`
- `migrateBatch(ctx, migSt, repo, globalBlocks, beforeBlocks, commitProgress func(txSt *state) error) error`
- `migrateFailed(ctx, migSt, repo) error` — --failed path; no state keys.
- Build migSt/repo in tests: `repo, _ := newPrepareRepository(ctx, db)`; `migSt := state{}; migSt.init(ctx, db)`.

## Orchestrator (T-017, orchestrator.go)
- `runRanges(ctx, migSt, repo, ranges []storedRange, batch int, p *progress) error`; `rangeError(r, err)` → `range %d [%d, %s]: %w` (`+inf` upper for last). On worker error migrationRun prints `Error: <err>` to stdout.
