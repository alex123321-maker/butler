# Tool Broker

The optional converter developer route uses a distinct fixed reviewed
`task.delegate_converter` contract and private admission client. The initial
context-only `task.delegate` contract retains empty child permissions. A developer
child receives only discover/develop/test; `converters.collect` is a separate
parent action after current immutable final/report and tested-package proof.
Original trials/activation remain ordinary parent grants. Routing constructors
do not enable the feature or grant authority.

The optional [reviewed procedural catalog](../../docs/architecture/reviewed-skills.md)
adds fixed local `skills.search/load/propose` operations. Skill text remains
DATA and actual tools retain current action/approval/budget/source admission.
Procedure bodies are deletion-aware derived sources; immutable skill history
contains hashes/refs rather than prose. Migration051 and the shared private
deletion anchor are required before enabling `BUTLER_SKILLS_ENABLED`. Actual
process/PG acceptance is documented in the [matrix](../../docs/architecture/reviewed-skills-acceptance.md).

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

Revision 2 browser preparation uses a narrow `local_browser_preparation_v1`
verifier derived from exact call args and persisted with the policy. It requires
matching session/current URL, increased observation version, successful local
acknowledgement and frozen network. This confirms only local browser state;
generic fill/type/click do not inherit it and external submission remains
unknown without recipient proof. Navigation/read revision 2 enforce GET/HEAD;
old contracts fail closed. The root unknown-effect budget guard is unchanged.

Reviewed local extension manifests are durable data in migration050. Owner
control is private and separate from ordinary tool execution. Exact versions,
operator profile review, fresh readiness and bounded current task attachment
filter scoped catalogs and are repeated before credentials/dispatch. The separate
Go executor receives only a dedicated runtime token and deferred authorized inputs;
fixed readiness GETs use existing owner/admin budgets and never completion proof.
See [extension lifecycle](../../docs/architecture/reviewed-extension-lifecycle.md).
Grant claim also rejects terminal/finalizing run state and checks current memory
source/transcript/privacy-watermark pins in the same transaction before budget
spend or dispatch. Broker reads the private deletion anchor volume read-only;
missing or stale restore history denies a new release. Existing spent journal
observations remain intact. See [versioned memory sources](../../docs/architecture/versioned-memory-sources.md).

Optional migration052 pure converters use owned `converters.*` lifecycle contracts
and dynamic exact `converter.<id>.convert`. They require current reviewed skill
search, an exact schema/profile/finite-limit ConverterPermission, source access,
budgets and ordinary journal grants. Development/checker sees only Need/synthetic
data; working input is released only after separately authorized Claim. Active
digest/generation and source guards are repeated before publication and use.
The production runtime has no network and uses a dedicated fixed Unix socket/token;
the private owner endpoint only lists/revokes, never bypasses activation. See
[generated converter contract](../../docs/architecture/generated-converters.md).

Optional `DelegationExecutor{Admissions, Next}` adds a Broker-owned control route
for the immutable `delegation.AdmissionContract()` startup contract. It requires
the actual private dispatch identity, exact schema/hash/target, no secrets and no
credential references. Other tools continue through `Next`. The private owning
Orchestrator service rechecks the executing grant and atomically admits an empty
tool-permission child; the receipt/effect acknowledgement confirms admission,
not child success. Service endpoints/tokens are operator-only composition; no
runtime orchestrates or receives the token and nothing is enabled by default.
`BUTLER_DELEGATION_ENABLED=true` requires PostgreSQL, the existing action-authority
token file and `BUTLER_DELEGATION_CONTROL_URL` naming exactly
`/internal/delegation`. Bootstrap then registers the fixed contract and Broker
control executor. Catalog and preparation deny use until the owning core
advertises composed readiness for a current task. Operator URL/settings do not
replace current dispatch, approval, source, owner and budget checks.
See [actual admission proof](../../docs/ai/delegation-admission-acceptance.md).

The distinct `BUTLER_CONVERTER_DELEGATION_ENABLED` opt-in also requires ordinary
delegation and enabled converters. Its operator-only
`BUTLER_CONVERTER_DELEGATION_CONTROL_URL` names exactly
`/internal/converter-delegation`. Bootstrap verifies immutable053/054 readiness,
registers only the fixed developer contract and composes its authenticated
control executor around the existing routes. Catalog and preparation require a
current task and the core's separate developer and converter lifecycle readiness;
these signals do not grant permissions or allow recursive child admission.

Optional controlled-browser resource admission now has explicit startup hooks:
`SetResourceAdmission`, the same-TX actionauth `SetDispatchClaimGuard` and
`NewResourceExecutor` around the actual private runtime router. Exact contract,
current run/task/source, grant and finite budget remain mandatory. Migration062
stores only hashed browser provenance and correlated dispatch metadata; foreign
session handles cannot claim or spend. Private tickets are gRPC metadata and do
not enter tools, approvals or results. This change leaves bootstrap/registry and
deployment disabled. All pieces, including the guarded actual runtime and
independent owner reconciliation, must be composed together before readiness.
See [resource adapter](../../internal/resourceexec/README.md).
