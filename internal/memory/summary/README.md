# Covered session summaries

This PostgreSQL store owns historical summaries with exact Transcript Store
source revision and message boundaries. `Tail` refuses unverifiable coverage;
`SaveCoveredSummary` refuses stale boundaries and changed sources. No summary
grants authority or replaces task constraints. See the
[memory contract](../../../docs/architecture/memory-model.md).

Unit tests cover invalid boundaries; PostgreSQL integration tests cover ordering,
late-worker publication, edits/deletions and immutable identities. Set
`BUTLER_TEST_POSTGRES_URL` to an isolated database to run integration checks.
