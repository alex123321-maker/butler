# Migrator

Utility binary for applying and rolling back Butler SQL migrations.

Behavior:
- serializes migration runs with a PostgreSQL advisory lock so concurrent migrator processes do not interleave schema changes

Dependencies:
- PostgreSQL connectivity via `internal/storage/postgres`
- migration files under `migrations/`

Configuration:
- required: `BUTLER_POSTGRES_URL` or `--postgres-url`
- optional: `BUTLER_POSTGRES_MIGRATIONS_DIR` or `--migrations-dir`

Local run:
- apply migrations: `go run ./apps/migrator --direction=up`
- roll back migrations: `go run ./apps/migrator --direction=down`

Testing:
- covered indirectly by storage and integration tests that execute migrations against PostgreSQL

Related docs:
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/memory-model.md`
- `docs/planning/butler-implementation-roadmap.md`
