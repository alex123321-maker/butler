# Migrations

SQL migrations live here.

- `001_sessions` creates the session store baseline.
- `002_runs` creates run lifecycle persistence per the lifecycle spec.
- `003_messages` creates transcript message storage.
- `004_tool_calls` creates normalized tool call persistence.
- `005_enable_pgvector` enables the vector extension for later retrieval work.
- `006_runs_idempotency_key` adds durable input-event deduplication on runs.
- `007_runs_metadata_json` adds durable run metadata storage.
- `008_memory_working` adds the durable working-memory snapshot store.
- `009_sessions_session_id` adds UUID session identifiers alongside stable session keys.
- `010_credentials` adds credential metadata storage.
- `011_memory_profile` adds profile-memory storage.
- `012_memory_episodes` adds episodic-memory storage.
- `013_credential_audit_logs` adds audit records for credential use decisions.
- `014_memory_schema_repairs` aligns confidence fields, vector dimensions, and episodic indexes with the memory model.
- `015_doctor_reports` adds persisted doctor diagnostic reports.
- `016_sessions_summary` adds durable session summaries for later context preparation.

Use `make migrate-up` to apply all migrations and `make migrate-down` to roll them back in reverse order.
