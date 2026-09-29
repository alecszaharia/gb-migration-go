---
created: "2026-09-29T00:00:00Z"
last_edited: "2026-09-29T00:00:00Z"
---

# Cavekit: Range State

## Scope
The persisted split of eligible global blocks into K contiguous ID ranges, and the per-range progress that makes a parallel migration run crash-safe and resumable. Covers split creation, split persistence, range boundaries, per-range watermarks, resume behavior, and the capacity of the state storage that holds these values.

This kit does NOT cover how workers execute ranges, how many run, connection capacity, error propagation, or progress reporting (see cavekit-parallel-execution.md).

"Eligible block" means the same eligibility used by the tool today: a global-block node with api_version = 2.

## Requirements

### R1: Split Creation
**Description:** When no split is stored, the tool computes a split of all eligible block IDs into K contiguous, non-overlapping ranges with balanced block counts.
**Acceptance Criteria:**
- [ ] Given N eligible blocks and K requested ranges with N >= K and no stored split, after split creation exactly K ranges exist
- [ ] After split creation, no eligible block ID falls into more than one range
- [ ] After split creation, every eligible block ID existing at split time falls into exactly one range
- [ ] After split creation, ranges are contiguous in ID order: sorting ranges by lower bound, each range's lower bound is greater than the previous range's upper bound, and no eligible ID lies between consecutive ranges
- [ ] After split creation with N >= K, every range's eligible block count is within ±1 of N/K
- [ ] Given N eligible blocks with 0 < N < K and no stored split, after split creation exactly N ranges exist and each contains exactly one eligible block
- [ ] Given zero eligible blocks and no stored split, no split is stored after the command runs
- [ ] Given zero eligible blocks and no stored split, the command output contains "Nothing to migrate"
- [ ] Blocks that are not eligible (not a global-block node, or api_version != 2) do not affect range counts
**Dependencies:** None

### R2: Split Persistence
**Description:** The split (K and every range's bounds) is persisted before any worker starts, and split creation is all-or-nothing.
**Acceptance Criteria:**
- [ ] After split creation and before any block of the run is migrated, the stored state contains K and the bounds of every range
- [ ] The stored K equals the number of stored ranges
- [ ] If the process is terminated at any point during split creation, the stored state afterwards contains either no split at all or a complete split (K plus all K ranges' bounds) — never a partial split
- [ ] No migrated block data is committed before the complete split is stored
**Dependencies:** R1

### R3: Open-Ended Last Range
**Description:** The last range (highest IDs) has no upper bound, so blocks created after the split belong to it.
**Acceptance Criteria:**
- [ ] The stored last range has no upper bound
- [ ] An eligible block with an ID greater than the highest eligible ID at split time, inserted after split creation and before the run completes, is assigned to the last range; it is processed in the current run if it exists when the last range's worker looks for its next batch, otherwise on the next run
- [ ] An eligible block with an ID greater than the highest eligible ID at split time, inserted after a completed run, is processed by the last range on the next run using the stored split
- [ ] No range other than the last contains IDs greater than its stored upper bound
**Dependencies:** R1, R2

### R4: Per-Range Watermark
**Description:** Each range has its own persisted last-processed ID, updated in the same transaction as the migrated data of that batch.
**Acceptance Criteria:**
- [ ] Each stored range has its own watermark value, independent of other ranges
- [ ] After a batch in a range commits, that range's stored watermark equals the highest block ID in that batch
- [ ] After a batch commits, the watermarks of all other ranges are unchanged by that commit
- [ ] If the process is terminated at any point, then for every range: every eligible block in the range with ID <= its watermark has committed migrated data or a failed-table record
- [ ] If the process is terminated at any point, then for every range: no eligible block in the range with ID > its watermark has committed migrated data
- [ ] When a block fails individually within a batch, it is recorded in the failed table with its error, and the range's watermark still advances past that block when the batch commits
**Dependencies:** R2

### R5: Resume From Stored Split
**Description:** If a split is already stored, it is reused unchanged regardless of the requested K, and each range resumes from its own watermark.
**Acceptance Criteria:**
- [ ] Given a stored split with K = S, running with requested K = S leaves the stored range count and bounds unchanged
- [ ] Given a stored split with K = S, running with requested K != S leaves the stored range count and bounds unchanged
- [ ] Given a stored split with K = S, running with requested K != S prints a notice stating that the stored K differs from the requested K
- [ ] Given a stored split with K = S, running with requested K = S prints no such notice
- [ ] On resume, the remaining work for each range is exactly the eligible block IDs greater than that range's watermark and less than or equal to its upper bound (unbounded for the last range)
- [ ] On resume, no block with ID <= its range's watermark is migrated again
**Dependencies:** R2, R3, R4

### R6: State Storage Capacity
**Description:** Every state key used by the tool is stored without truncation, and state storage created by earlier versions is upgraded in place idempotently.
**Acceptance Criteria:**
- [ ] For every state key the tool writes, reading it back returns the exact same key with no truncation
- [ ] Given state storage created by an earlier version (keys limited to 25 characters), after the tool runs its upgrade, all keys the tool writes are stored without truncation
- [ ] Given state storage created by an earlier version containing existing rows, after the upgrade all pre-existing rows are present with unchanged keys and values
- [ ] Running the upgrade a second time completes without error
- [ ] Running the upgrade a second time leaves all rows and their values unchanged
**Dependencies:** None

## Out of Scope
- Honoring the legacy `latest_processed_block_id` watermark; it is ignored
- A reset command; creating a new split requires manually clearing stored state
- Re-splitting remaining work of an in-progress split
- Failed-table semantics (unchanged from today)
- Worker execution, worker count, connection capacity, error propagation, and progress output

## Cross-References
- See also: cavekit-parallel-execution.md — consumes R1, R3, R5 for work assignment and R4 on every batch commit

## Changelog
- 2026-09-29: Initial draft from approved design
- 2026-09-29: Review clarification — blocks inserted during a run are processed in that run only if present when the last worker looks for its next batch
