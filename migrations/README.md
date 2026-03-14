# Migrations

SQL migrations live here.

- `001_sessions` creates the session store baseline.
- `002_runs` creates run lifecycle persistence per the lifecycle spec.
- `003_messages` creates transcript message storage.
- `004_tool_calls` creates normalized tool call persistence.
- `005_enable_pgvector` enables the vector extension for later retrieval work.
- `006_runs_idempotency_key` adds durable input-event deduplication on runs.
- `007_runs_metadata_json` adds durable run metadata storage.

Use `make migrate-up` to apply all migrations and `make migrate-down` to roll them back in reverse order.
