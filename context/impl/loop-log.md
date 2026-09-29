# Loop Log

Build site: context/plans/build-site.md

### Iteration 1 — 2026-09-29
- Setup: kits/build site/schema.sql (mysqldump --no-data brizy-cms, tool tables stripped) committed 775e8a1 = TIER_START_REF tier 0. ck:task-builder agent def broken (tools frontmatter) → golang-pro fallback. Worktrees spawn at stale base 7f01d75 → agents ff-merge main first.
- T-001: workers flag — DONE. Files: command.go, command_test.go, migration.go. Build P, Tests P.
- T-006: progress reporter — DONE. Files: progress.go, progress_test.go. Build P, Tests P (-race).
- T-002/T-003/T-004: split, state upgrade, range queries — DONE. Files: split.go, split_test.go, state.go, state_test.go, repository.go. Build P, Tests P.
- Next: T-005 (running), T-007+T-008 (running), then T-009..T-012.
- T-007: split unit tests — DONE. T-008: pool K+1 — DONE. Files: split_test.go, mysql.go, mysql_test.go, migration.go. Build P, Tests P (35).
- T-005: integration harness — DONE. Files: hooks.go, harness_test.go, harness_snapshot_test.go, harness_run_test.go, harness_unit_test.go, harness_integration_test.go. Build P, Tests P (44, DSN set, -race). API: context/impl/harness-api.md. Tier 0 complete.
- T-011: golden baseline (synthetic fixture) — DONE. T-012: rejected -w writes nothing — DONE. Files: scripts/capture_baseline.sh, golden_helpers_test.go, golden_capture_integration_test.go, testdata/golden/*.json, command_integration_test.go. Build P, Tests P (50 w/ DSN).
- T-009: split/watermark persistence — DONE. T-010: upgrade integration tests — DONE. Files: state.go, state_test.go, state_integration_test.go. Build P, Tests P (81, -race, DSN). Tier 1 complete. Codex unavailable → tier gate skipped.
- T-013/T-014: ensureSplit + runRange/migrateBatch refactor — DONE. Files: migration.go, harness_integration_test.go, range_worker_smoke_integration_test.go. Build P, Tests P (85). Golden -w 1 + --failed match. Tier 2 complete; codex gate skipped.
