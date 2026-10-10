# Reviewed capabilities

Strict declarative manifest validation, immutable version history, review,
configuration, health, task attachment and scoped discovery belong here.
PostgreSQL is authoritative; public installation text remains DATA.

`BindAction` denies stale capabilities before credential inspection;
`CheckActionTx` repeats admission within irreversible Broker dispatch.
`ManifestByHash` reads historical provenance and grants no authority.
The only out-of-task egress is owner-only fixed budgeted readiness GET.

See [the lifecycle contract](../../docs/architecture/reviewed-extension-lifecycle.md).
`projectfixture` prepares only one synthetic README patch and never modifies a
real repository or runs a build. External artifact creation uses ordinary commit
and recipient-receipt semantics.

Run `go test ./internal/extensions/...` on the isolated lab with
`BUTLER_TEST_POSTGRES_URL` pointing to a separate `*_test` database. Fixtures
create private schemas and use MaxConns=2; no production credentials are needed.
