---
created: "2026-09-29T00:00:00Z"
last_edited: "2026-09-29T00:00:00Z"
---

# Cavekit: Parallel Execution

## Scope
Running the stored split concurrently: worker-count configuration, connection capacity, one worker per range, fatal error propagation, aggregate progress reporting, completion correctness, and preservation of the existing `--failed` mode.

This kit does NOT cover how the split is created, stored, or resumed, or how per-range watermarks are persisted (see cavekit-range-state.md).

K denotes the configured worker count (`--workers`), which is also the requested number of ranges.

## Requirements

### R1: Worker Count Option
**Description:** A new option `--workers` / `-w` sets the worker count, configurable the same way as existing options (command-line flag and environment), defaulting to 4.
**Acceptance Criteria:**
- [ ] Running with `--workers 3` sets the worker count to 3
- [ ] Running with `-w 3` sets the worker count to 3
- [ ] Setting the worker count through the environment, using the same convention as existing options, sets the worker count to that value
- [ ] Running with no flag and no environment value sets the worker count to 4
- [ ] Running with a worker count of 0 exits with a non-zero status and an error message
- [ ] Running with a negative worker count exits with a non-zero status and an error message
- [ ] When the worker count is rejected, no database write has occurred (stored state, migrated data, and failed table are unchanged)
**Dependencies:** None

### R2: Connection Capacity
**Description:** The database connection pool permits at least K+1 concurrent connections so no worker waits on another for a connection.
**Acceptance Criteria:**
- [ ] With worker count K, the configured maximum number of open connections is >= K+1
- [ ] With worker count K and K ranges, all K workers can hold an open transaction simultaneously without any worker blocking on connection acquisition
**Dependencies:** R1

### R3: One Worker Per Range
**Description:** Each range with remaining work gets one worker; all workers run concurrently. Each worker processes its range in ascending ID order in batches of `--batch` size, each batch in its own transaction with today's per-block failure isolation.
**Acceptance Criteria:**
- [ ] For a split with R ranges that all have remaining work, R workers run concurrently
- [ ] Each worker only processes block IDs belonging to its assigned range
- [ ] Within a range, batches are processed in ascending block ID order
- [ ] Each batch contains at most `--batch` blocks
- [ ] Each batch is committed in its own transaction, separate from other batches and other workers
- [ ] When one block in a batch fails, that block has no committed migrated data, it is recorded in the failed table with its error, and the other blocks of the batch commit
- [ ] A range with no remaining work finishes without migrating any block
- [ ] A worker finishes as soon as a lookup for its next batch finds no eligible block above its watermark within its range (including the open-ended last range); it does not wait or poll for new blocks
**Dependencies:** range-state R1, range-state R3, range-state R4, range-state R5, R2

### R4: Fatal Error Handling
**Description:** A fatal error in any worker (anything other than a single-block failure) cancels all workers, leaves no batch partially committed, exits non-zero naming the range, and allows resume from each range's watermark.
**Acceptance Criteria:**
- [ ] When a fatal error occurs in one worker, all other workers stop starting new batches
- [ ] After a fatal error, no batch in any range is partially committed: each batch's migrated data and watermark update are either fully committed or fully absent
- [ ] After a fatal error, the command exits with a non-zero status
- [ ] After a fatal error, the error output includes the affected range's index and its lower and upper bounds (upper shown as open-ended for the last range)
- [ ] After a fatal error, rerunning the command resumes every range from its own stored watermark
- [ ] A single-block failure (recorded in the failed table) does not cancel any worker
**Dependencies:** R3, range-state R4, range-state R5

### R5: Aggregate Progress
**Description:** A single progress line covers all workers, showing processed/total, percentage, and overall blocks per second.
**Acceptance Criteria:**
- [ ] During a run, exactly one progress line is shown for all workers combined (not one per worker)
- [ ] The progress line shows processed count and total count
- [ ] The progress line shows percentage complete
- [ ] The progress line shows overall throughput in blocks per second
- [ ] The total equals the sum of remaining eligible blocks across all ranges at run start
- [ ] At successful completion, the processed count is greater than or equal to the total (it exceeds the total only when the last range picked up blocks inserted during the run)
**Dependencies:** R3, range-state R5

### R6: Correctness Equivalence
**Description:** After a full run with any K, every eligible block is migrated exactly once or recorded in the failed table; K=1 behaves like the existing sequential tool.
**Acceptance Criteria:**
- [ ] After a full successful run with K in {1, 2, 4, 8}, every eligible block has either migrated data or a failed-table record
- [ ] After a full successful run with any K, no eligible block has been migrated more than once (no duplicate migrated records)
- [ ] After a full successful run with any K, no eligible block is missing both migrated data and a failed-table record
- [ ] Given the same source data, the set of migrated blocks and the set of failed blocks after a run with K=1 are identical to those after a run with K=4
- [ ] Given the same source data and starting from empty migration state, a run with K=1 produces the same migrated data and failed-table contents as the existing sequential tool
**Dependencies:** R3, R4, range-state R1, range-state R4

### R7: Failed Mode Unchanged
**Description:** `--failed` retry mode stays sequential and ignores both worker count and split state.
**Acceptance Criteria:**
- [ ] Running with `--failed` processes failed-table blocks with a single worker regardless of `--workers`
- [ ] Running with `--failed` does not create a split
- [ ] Running with `--failed` does not read or modify any stored split or per-range watermark
- [ ] Running with `--failed` produces the same migrated data and failed-table contents as the existing tool for the same input
**Dependencies:** None

## Out of Scope
- Parallelizing `--failed` mode
- Per-block write speedups (multi-statement writes, dropping savepoints)
- Adjusting K at runtime
- Signal handling beyond current behavior
- Split creation, persistence, resume, and watermark storage (covered by range-state)

## Cross-References
- See also: cavekit-range-state.md — depends on R1, R3, R5 for work assignment and R4 for committing progress on every batch

## Changelog
- 2026-09-29: Initial draft from approved design
- 2026-09-29: Review clarifications — worker stops when no eligible block remains (no polling), progress processed may exceed start total, fatal error names range index+bounds, K=1 equivalence assumes empty state
