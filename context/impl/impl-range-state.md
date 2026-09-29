---
created: "2026-09-29T00:00:00Z"
last_edited: "2026-09-29T00:00:00Z"
---
# Implementation Tracking: range-state

Build site: context/plans/build-site.md

| Task | Status | Notes |
|------|--------|-------|
| T-002 | DONE | split.go: idRange{Index,Lower,Upper *int64}, computeSplit (min(k,N) ranges, first N%K get +1, last Upper=nil), contains. Sanity test split_test.go |
| T-003 | DONE | state.go: CREATE set_key VARCHAR(64); upgrade() checks information_schema CHARACTER_MAXIMUM_LENGTH, ALTER only if <64; key consts/helpers splitKKey, rangeLowerKey/UpperKey/WatermarkKey; state_test.go asserts keys <=64 for K<=9999. Note: value column still INT (signed 32-bit) |
| T-007 | DONE | split_test.go table-driven TestComputeSplit: N=10/K=3 (4,3,3), N=K, N<K, K=1, N=0, sparse, huge gaps, N=1000/K=7; asserts min(K,N) count, exact-once coverage via contains, no overlap, contiguity, ±1 sizes, only last Upper nil, !contains(upper+1), last contains(max+1000) |
| T-004 | DONE | repository.go: getAllEligibleIds, getGlobalBlocksIdsInRange, getRemainingCountInRange on shared eligibleFrom predicate, `? IS NULL OR d.id <= ?`; rows.Close fix + scanIds helper. Manually SQL-checked on throwaway DB |
| T-005 | DONE | Harness (shared w/ parallel-execution): hooks.go + harness_*_test.go; see context/impl/harness-api.md. Self-test runs sequential tool on eligible/failing/ineligible seeds. -race P with DSN |
| T-009 | DONE | state.go: storedRange{idRange;Watermark}, loadSplit (assembleSplit/validate, errors on inconsistent), storeSplit one tx w/ callHookSplitStoreStep before each write + before commit (splitStoreWrites+1 hook calls), watermark=lower-1, updateRangeWatermark via tx-bound stmt; value column BIGINT (upgrade idempotent) |
| T-010 | DONE | state_integration_test.go: legacy VARCHAR(25)/INT + latest_processed_block_id=123 → init → all keys incl range_9999_watermark exact, legacy row kept, 2nd init identical; plus round-trip K=1,3,8, per-step hook error/cancel → no partial split, tx watermark isolation, BIGINT 5e9 |
| T-013 | DONE | migration.go ensureSplit(ctx, db, migSt, repo, requestedK): loadSplit → notice on K mismatch; else getAllEligibleIds → `Nothing to migrate` (no store) or computeSplit+storeSplit before workers. Legacy getState path removed from migrationRun |
| T-014 | DONE (shared w/ PE R3) | migrateBatch(ctx, migSt, repo, gbs, beforeBlocks, commitProgress); runRange(ctx, migSt, repo, r, batch, p): ctx check → hookBatchStart → range lookup → empty returns → migrate → watermark update + hookBeforeBatchCommit in tx. migrateFailed replaces iterateThroughBatches. Interim sequential range loop. Golden quick check P (-w 1 + --failed). range_worker_smoke_integration_test.go |
| T-019 | DONE (shared w/ PE R3) | range_worker_integration_test.go: TestRangeWorkerBatches (asc/in-range/≤3/watermark=max/others unchanged/failing block recorded+passed), TestRangeWorkerBatchOwnTx (batch2 pre-commit error keeps batch1), TestRangeWorkerNoRemainingWork, TestRangeWorkerReturnsAfterEmptyLookup (open range, exactly batches+1 starts) |
