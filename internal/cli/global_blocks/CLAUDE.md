# global_blocks

Implements:
- cavekit-range-state.md R1 (Split Creation), R2 (Split Persistence), R3 (Open-Ended Last Range), R4 (Per-Range Watermark), R5 (Resume From Stored Split), R6 (State Storage Capacity)
- cavekit-parallel-execution.md R1 (Worker Count Option), R3 (Concurrent Range Workers), R4 (Fatal Error Handling), R5 (Aggregate Progress), R6 (Correctness Equivalence), R7 (Failed Mode Isolation)

Build tasks: T-001–T-007, T-009–T-029 (context/plans/build-site.md)

Integration tests need `GB_MIGRATION_TEST_DSN` (e.g. `root:nopassword@tcp(127.0.0.1:3306)/`); harness API: context/impl/harness-api.md.
