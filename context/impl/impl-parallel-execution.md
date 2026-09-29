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
