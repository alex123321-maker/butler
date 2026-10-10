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
isolated lab database with migrations 001–034 applied.
