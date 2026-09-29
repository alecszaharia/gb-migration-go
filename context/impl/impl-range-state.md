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
