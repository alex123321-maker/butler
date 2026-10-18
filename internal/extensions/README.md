# Reviewed capabilities

Strict declarative manifest validation, immutable version history, review,
configuration, health, task attachment and scoped discovery belong here.
PostgreSQL is authoritative; public installation text remains DATA.

`BindAction` denies stale capabilities before credential inspection;
`CheckActionTx` repeats admission within irreversible Broker dispatch.
`ManifestByHash` reads historical provenance and grants no authority.
`ManifestByHashTx` performs the same immutable lookup on an existing transaction
without borrowing a second connection or acquiring capability/authority locks.
The only out-of-task egress is owner-only fixed budgeted readiness GET.

See [the lifecycle contract](../../docs/architecture/reviewed-extension-lifecycle.md).
`projectfixture` prepares only one synthetic README patch and never modifies a
real repository or runs a build. External artifact creation uses ordinary commit
and recipient-receipt semantics.

The fixture manifest documents URL composition as package DATA: inspect uses
the current task's exact project URL (`/project` or `/project/{resource}`);
prepare appends `/patches`; artifact checks append
`/patches/{artifact_id}/checks` to the inspected project URL. Resource names and
artifact identities come from current inputs and observations. These descriptions
do not grant access or change Broker/runtime dispatch or receipt validation.

Run `go test ./internal/extensions/...` on the isolated lab with
`BUTLER_TEST_POSTGRES_URL` pointing to a separate `*_test` database. Fixtures
create private schemas and use MaxConns=2; no production credentials are needed.
