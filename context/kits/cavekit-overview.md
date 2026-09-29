---
created: "2026-09-29T00:00:00Z"
last_edited: "2026-09-29T00:00:00Z"
---

# Cavekit Overview

## Project
BrizyGBMigration is a CLI that migrates Brizy global blocks from a legacy MySQL schema to a new one. This cavekit set covers running the migration in parallel: splitting eligible blocks into K ranges with persisted, crash-safe per-range progress, and executing those ranges concurrently with one worker each, while preserving existing per-block failure isolation and the sequential `--failed` retry mode.

## Domain Index

| Domain | Cavekit File | Requirements | Status | Description |
|--------|--------------|--------------|--------|-------------|
| Range State | cavekit-range-state.md | 6 (34 criteria) | DRAFT | Persisted split into K ranges and per-range watermarks for crash-safe resume |
| Parallel Execution | cavekit-parallel-execution.md | 7 (38 criteria) | DRAFT | Worker-count option, concurrent per-range workers, fatal error handling, aggregate progress |

Total: 13 requirements, 72 acceptance criteria.

## Cross-Reference Map

| From | To | Relationship |
|------|----|--------------|
| parallel-execution R3 | range-state R1, R3, R5 | Work assignment: which ranges exist and which IDs remain per range |
| parallel-execution R3 | range-state R4 | Every batch commit advances that range's watermark in the same transaction |
| parallel-execution R4 | range-state R4, R5 | Fatal-error rerun resumes each range from its watermark |
| parallel-execution R5 | range-state R5 | Progress total = remaining eligible blocks across ranges |
| parallel-execution R6 | range-state R1, R4 | Exactly-once correctness relies on non-overlapping ranges and atomic watermarks |
| range-state (all) | parallel-execution | Consumer of split and watermark state |

## Dependency Graph

```
range-state  ──►  parallel-execution
```

range-state is implemented first; parallel-execution depends on it. No cycles.
