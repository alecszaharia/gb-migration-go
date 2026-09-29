---
created: "2026-09-29T00:00:00Z"
last_edited: "2026-09-29T00:00:00Z"
---
# Implementation Tracking: parallel-execution

Build site: context/plans/build-site.md

| Task | Status | Notes |
|------|--------|-------|
| T-001 | DONE | command.go: --workers/-w default 4, viper bind (WORKERS env); PreRunE rejects <1 with `invalid worker count N: must be >= 1` before NewDB. command_test.go covers flag/short/env/default/0/-1 |
| T-006 | DONE | progress.go: newProgress(total,w,now), atomic add, render `\033[F\033[2KProgress: %.2f%% (%d/%d) | %.2f blocks/s`, startRendering(interval) single goroutine, stop() final render. progress_test.go fake clock + 16x1000 concurrent adds, -race clean |
| T-008 | DONE | mysql.go: NewDB(dsn, workers) + configurePool MaxOpen=MaxIdle=max(workers+1,20); mysql_test.go K∈{1,4,50} asserts MaxOpenConnections>=K+1; migration.go passes viper workers |
| T-011 | DONE | scripts/capture_baseline.sh builds 7c5eea1 in temp worktree, runs gated TestCaptureGoldenBaseline (GB_CAPTURE_GOLDEN_BIN) → testdata/golden/normal.json, failed.json. Fixture = deterministic synthetic seedGoldenFixture (no production fixture DB available). Helpers in golden_helpers_test.go |
| T-012 | DONE | command_integration_test.go TestRejectedWorkerCountWritesNothing: -w 0, -w -2, WORKERS=0 env, -w 0 --failed → error non-empty, takeSnapshot unchanged (migrated/failed/state non-empty beforehand) |
