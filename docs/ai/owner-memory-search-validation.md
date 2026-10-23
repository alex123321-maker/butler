# Owner Web memory search — local validation, 2026-10-23

Implemented contract: [owner hybrid search](../architecture/versioned-memory-sources.md#owner-web-hybrid-search-2026-10-23).

## Passed

- Web typecheck, ESLint/Stylelint and production Nuxt build.
- Playwright `memory-explorer.spec.ts` and `mind.spec.ts`: 35 passed. Includes
  explicit hybrid submission, lexical typing, fallback, cursor continuation,
  deduplication, cancellation of an older query and three homepage previews.
- Linux Go tests against isolated PostgreSQL 17 with pgvector: 13 passed,
  no skips in the selected groups:
  - catalog: `TestOwnerBrowse` (4);
  - semantic: `TestOwnerSearch` (4);
  - orchestrator API: `TestOwnerSearch|TestMemoryOwnerBrowse` (4);
  - budget: `TestOwnerQuery` (1).
- Desktop and mobile demo screenshots inspected; both local preview endpoints
  returned HTTP 200 after the final build.

Database tests use `BUTLER_TEST_MEMORY_POSTGRES_URL` pointing to the temporary
`owner_search_unicode_test` database, UTF-8 encoding and `C.UTF-8` locale.
Fixtures apply all migrations in isolated schemas. The initial `C` locale did
not lowercase Cyrillic; recreating the disposable database with `C.UTF-8`
resolved that environment failure without weakening the matching test.
Cross-compiled Linux tests used a package-specific `-trimpath` mapping for
`internal/memory/memorytest`, so its runtime source path locates real migrations.

## Coverage limits

The embedding transport is deterministic in integration tests. They exercise
real vector SQL, authentication, current-source checks, budget reservation and
fallback; they do not measure native EmbeddingGemma2 relevance or latency.
Native inference was not started on this PC. The Web backend uses the existing
pinned local semantic runtime when configured and an active compatible index
exists. Search covers catalog sources and indexed fragments, not an independent
scan of every raw transcript row.

The clickable preview on port 3002 uses fictional in-browser records and visibly
labels simulated semantic matching. It does not connect to the owner's memory.

Broader Windows package tests hit existing Unix permission/fsync assumptions;
the selected storage tests were therefore executed on Linux in local WSL.
Docker Desktop could not start because of its stale `dockerInference` socket;
automatic approval rejected socket deletion. An isolated temporary PostgreSQL
under WSL `/run` provided validation without altering that socket or deploying
to a remote host. The temporary database is stopped after validation.
