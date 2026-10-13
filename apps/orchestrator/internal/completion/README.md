# Completion

Owns immutable run-result persistence and durable auxiliary jobs. `Coordinator`
holds the fenced session ownership guard and delegates transaction writes to
typed session/run/task/artifact/transcript repositories. It commits the required
artifact, transcript, unverified task result, final result, run completion,
outbox and auxiliary jobs together. Persistence recovery saves the frozen final
answer and has no model/tool dependency. Task completion requires separate owner
verification.

See [atomic result/delivery contract](../../../../docs/architecture/atomic-result-delivery.md).
Integration tests require `BUTLER_TEST_COMPLETION_POSTGRES_URL`; run them on the
isolated lab database with all checked-in migrations applied. Result persistence
requires migration 034; the configured permanent effect reader requires 035.
The PostgreSQL fixture limits the pool to two connections to check ownership
guard/evidence-read/transaction sequencing.

For an explicitly admitted `verified_task` flow, Completion keeps technical finals,
artifacts, unverified results and auxiliary jobs but omits the external per-run
outbox. Only Task's current owner verification/completion transaction creates the
one semantic root notice. Legacy `attempt` delivery is unchanged.
