# Tool Broker

Every dispatch now uses `PrepareToolCall` and `AuthorizeToolCall` for the complete
action, followed by `ExecuteToolCall.authorization_id`. PostgreSQL retains exact
intent, grant state and bounded mandates. Approval is revalidated before secrets
and dispatch; uncertain effects cannot reuse a grant. Reflected resolved secrets
are masked before results reach the model. See
[action authorization](../../docs/architecture/action-authorization-contract.md).

Private execution RPCs and `/internal/action-mandates` require a service token
from `BUTLER_ACTION_AUTHORITY_TOKEN_FILE`. Configure `BUTLER_ACTION_CONTEXT_URL`
to the authenticated orchestrator authority endpoint. Missing wiring fails
closed. Compose mounts the token as a secret, outside model/tool arguments.

Browser contracts now separate independent `browser.read` from run-owned
`browser.open`/`browser.close` and explicit continuation. Stateful effects require
the latest observation version and expected current URL; the runtime validates
actual state, ownership and fixed origin before effects. Cookie/storage inputs
use typed credential refs only, with their separate approvals preserved. See
[`browser-session-contract.md`](../../docs/architecture/browser-session-contract.md).

Tool Broker service for Butler tool validation, policy enforcement, credential mediation, and runtime routing.

Current state:
- gRPC exposes catalog/schema inspection and private exact-action preparation, authorization and execution
- in-memory Tool Registry is loaded from `configs/tools.json`
- schema validation is performed against tool `input_schema_json`
- execution routing dials the per-tool `runtime_target` over the internal runtime gRPC contract
- credential metadata foundation lives in `internal/credentials` with PostgreSQL-backed alias records and policy authorization helpers
- credential refs are resolved in the broker via `secret_ref` values and forwarded to runtimes through `resolved_credentials`
- the default Compose stack wires PostgreSQL into the broker so credential policy checks and audit-backed metadata lookups are active by default

Expected dependencies:
- generated `toolbroker` gRPC bindings
- generated `runtime` gRPC bindings
- typed config from `internal/config`
- credential metadata and future secret resolution layers

Configuration:
- current typed config loader supports `BUTLER_TOOL_REGISTRY_PATH` and `BUTLER_TOOL_DEFAULT_TARGET`
- placeholder registry file lives at `configs/tools.json`
- shared service settings come from `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, and `BUTLER_LOG_LEVEL`
- credential metadata lookup uses `BUTLER_POSTGRES_URL`; the current secret resolver supports `env://ENV_VAR_NAME` references

Local run:
- `go run ./apps/tool-broker`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and gRPC on `BUTLER_GRPC_ADDR`
- Compose stack wires the broker to `tool-http`, `tool-browser`, and `tool-doctor` over the internal runtime network
- web retrieval tools can now route to `tool-webfetch`
- `single_tab.*` contracts may exist in disabled state before the local browser runtime is available

Testing:
- run `go test ./apps/tool-broker/...`

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`

External effects use migration035's PostgreSQL journal. Preparation accepts an
optional trusted logical slot; grants commit only under live ownership/task
locks. Runtime completion without supported external evidence returns
`effect_unknown`, including repeated RPCs after restart, and never enables
replay. Private `/internal/action-effects` supplies typed journal snapshots and
reconciliation from an independently captured authorized read. Defaults declare
no recipient idempotency guarantee. See
[the effect contract](../../docs/architecture/external-effect-journal.md).

Migration037 admission reserves permanent-effect call/time/egress allowance in
the same PostgreSQL transaction as the irreversible grant/journal claim, after
live task and assigned authority checks. Normalized loops and unknown non-read
effects block fresh non-read dispatches. Runtime deadline is capped by every scope
deadline. Completed output must pass the registry schema and contain no error
before becoming evidence. Budget rejection returns `execution_stopped` with a
typed reason and invokes no runtime. Configure the same operator
`BUTLER_EXECUTION_BUDGET_POLICY_JSON` as Orchestrator. See
[execution budgets](../../docs/architecture/execution-budget-contract.md).

Current owner goal/purpose-policy and active-hours deadlines also cap the exact
grant and runtime context. Claim rechecks the database clock after budget waits
on the already held authority rows; expiry rolls back allowance without spending
the grant or contacting a runtime. Original budget periods remain immutable.

The release guard treats arbitrary network paths/queries/headers/body as potentially private. Reads outside an explicit operator domain declaration require exact owner approval or a live assigned owner-goal mandate with confirmed purpose/resource scope. Changed arguments cannot reuse the canonical proof; revoked authority is denied before secret inspection/DNS. This composes with the037 immutable byte reservation, not a model-controlled privacy scanner. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).
