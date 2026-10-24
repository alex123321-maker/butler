# Tool Broker

The optional migration096 credential vault installs canonical `credentials.*`
search/access/create/OTP controls and resolves owner-approved request refs to
independent encrypted fields. `BUTLER_CREDENTIAL_KEY_FILE` must reference the
same mounted key as Core. There is no env-secret compatibility resolver. Account
site hints do not restrict destinations; the approved request binds the actual
browser origin/session, fields, task versions and record revision. These local
controls grant no submission authority. See [vault operations](../../internal/credentials/README.md).

Conversation repetition is scoped to the original Input event using existing
permanent effect/run provenance; a new owner request may repeat a catalog read.
This does not reset conversation/day spending, task-tree loops or session-wide
unknown-write protection. Missing provenance remains conservative. See
[the budget contract](../../docs/architecture/execution-budget-contract.md).

Dormant `CommunicationExecutor` implements fixed owning control dispatch for
the strict `communication.*` contracts. It forwards only actual dispatched action
identity and attempts a read-only receipt lookup after lost ACK. It does not
register the family in Bootstrap; no current provider receives these schemas.
Exact approval/authority/budget checks remain mandatory. See
[the kernel boundary](../orchestrator/internal/communication/README.md).

Operator `disable_conversation_model_turns:true` disables only the conversation
session/day model-turn comparison while preserving every charge and all other
bounds. Match the Core setting; existing scope limits/history are not rewritten.

Operator budget policy `disable_owner_day:true` omits the aggregate owner/day
scope for new reservations. It must match Orchestrator. Task/mandate/delegation
limits, authority and existing spending remain enforced; see
[the budget contract](../../docs/architecture/execution-budget-contract.md).

Fixed memory arguments are checked before effect preparation and dispatch, using
the same strict decoder as execution. Out-of-range message limits fail validation
without producing an unknown effect. memory.open can return bounded retained
evidence authorized by the Catalog's current source/privacy guard.

Correlated explicit validation failures for HTTP/WebFetch read operations
preserve their bounded diagnostic (512 bytes) and details (1024 bytes), for
example an HTTP status or unsupported MIME, when no failed-result body is present. They
remain `failed`, with the existing `effect_unknown` journal state and
`retryable:false`; this is no completion proof or replay permission. Lost RPCs,
malformed success schemas, mismatched identities and write failures retain the
generic unknown-effect response. See the [web result contract](../../docs/architecture/tool-runtime-adr.md#web-result-correctness).

Taskless conversations exclude converter lifecycle/catalog and collection reads
before their task-only guards, so enabling converters preserves ordinary catalog
discovery. Core/Broker process checks exercise discovery with both skills and
converters enabled; converter execution still requires current task authority.
Ordinary tasks without a converter assignment also skip converter-only guards
while retaining permitted core tools such as memory reads.

Optional candidate proposals support the exact private Compose destination
`http://orchestrator:8080/internal/memory-candidate`; arbitrary remote plain HTTP
is denied. This matches the read/fact clients and preserves authenticated dispatch.

The reviewed-skills execution wrapper preserves earlier local handlers, including
discovery, instead of replacing them with the runtime router. The actual
Core/Broker discovery integration also runs with skills enabled.

Optional operator-only `BUTLER_TOOL_DISCOVERY_ENABLED=true` registers immutable local `tools.search` and `tools.load` reads in the ordinary action/claim/budget path. Search cards cover the full permitted catalog; exact loading does not execute tools or grant rights. Enable the matching Orchestrator flag. See [tool discovery](../../docs/architecture/tool-discovery.md).


Optional `BUTLER_SCHEDULES_ENABLED=true` registers immutable `schedule.*`
contracts, filtered by current owning readiness and executor scope. The fixed
operator `BUTLER_SCHEDULES_CONTROL_URL` routes actual dispatched calls to
Orchestrator `/internal/schedules`, using existing private service authentication,
exact grants, input claims and shared budgets. No credential values or owner IDs
are model arguments. Enable the matching Orchestrator flag and migration085;
see the [implemented contract](../../docs/architecture/agent-schedules.md).
Enabled controls are standard in ordinary owner-goal work even when omitted from
the mandate's runtime tool/resource lists. Exact owning contracts and explicit
denials/effect/authority/approval limits remain required; no test grant is needed.

The optional `memory.facts` contract revision4 separates explicit validity expiry
from verification deadlines. It preserves the bounded input/output schema and
ordinary authorization/source pins. The general declaration adds no source
values or inference authority; temporal release qualification remains false.

Optional operator-only `BUTLER_MEMORY_READ_ENABLED=true` registers the fixed
`memory.search`, `memory.open`, `memory.neighbors` contracts through RegisterOwned.
Set `BUTLER_MEMORY_READ_CONTROL_URL` to the authenticated owning Orchestrator route
ending `/internal/memory-read`; enable the same flag in Orchestrator. Reads use
normal grants/journal/shared budgets and current source pins. No arbitrary SQL,
paths, URL fetches or credential inputs are accepted. See
[read contract](../../internal/memory/readcontrol/README.md).

`BUTLER_NETWORK_ALLOWLISTS_JSON` is an optional, startup-only destination cap
for `http`, `browser` and `web` tool classes. For example,
`{"http":["api.example.com"],"browser":["*.example.com"]}` restricts supplied
`url`, `expected_url` and every `urls` entry before preparation, authorization
and dispatch. An omitted class has no additional cap; an explicit `[]` denies
all supplied destinations. Domains are exact unless prefixed with `*.`.
Invalid policy prevents startup. The class cap and the original contract
declaration are checked independently, including disjoint and wildcard lists.
The cap never replaces `AllowedDomains`, grants release permission or bypasses
exact action/credential/resource authority. Session-bound operations retain their
existing current-resource guards and runtime network restrictions.

Operator Doctor checks use the ordinary exact-action preparation, authorization
and one-use grant path with a real durable task/run/session lease. There is no
operator bypass. `doctor.check_system` revision 2 declares a read-only check with
`read_result` evidence; this does not establish memory answer quality. See the
[memory/Doctor runbook](../../docs/operations/memory-observability.md).

The owned converter catalog advertises `converters.collect` only after the
current Store verifies an actual immutable tested child return. A queued
developer receipt ends the current parent attempt; collection resumes through
the ordinary continuation. Listing grants no source or action, and Prepare/Claim
repeat all current authority, ownership, deletion and return checks.

Workspace execution has an exact optional Server/router adapter in
`internal/broker/workspaces.go`: canonical create/read/write DATA contracts,
same-transaction current input/source/worker/budget/resource admission and one-use
private runtime metadata. `ResourceAdmissions` prevents workspace and browser
guards from substituting for each other. Task discovery/preparation requires
current `workspace_ready`. Operator-only `BUTLER_WORKSPACE_ENABLED` now registers
the exact contracts after authenticated installed runtime/schema/anchor readiness.
It requires a stable profile, fixed loopback or private Unix target and service
token file. Raw workspace registry entries fail startup, including when disabled.
Workspace-only composition hides browser tools without their required resource
adapter; the exact local guarded selection request retains its separate path.
The actual Broker/runtime/files acceptance is described in
[workspace dispatch](../../docs/ai/workspace-dispatch-acceptance.md). Production
rollout and owner reconciliation remain pending. See also
[application setup](../tool-workspace/README.md) and
[bootstrap proof](../../docs/ai/workspace-bootstrap-acceptance.md).

Optional operator-only `BUTLER_MEMORY_CANDIDATE_ENABLED` registers the canonical
local `memory.propose_candidate` control after exact authorization/permanent
dispatch/shared budgets. It requires PostgreSQL, private action authority and a
fixed `BUTLER_MEMORY_CANDIDATE_CONTROL_URL` ending `/internal/memory-candidate`.
Current owning readiness and task proof gate discovery/preparation. This route
accepts no credential material and never writes accepted memory directly. See
[the private control](../../internal/memory/candidatecontrol/README.md). Default false.

Optional `BUTLER_GUARDED_TAB_RUNTIME_ENABLED` now composes canonical guarded-1
status/click contracts, exact private runtime readiness, current selection/source
checks, shared budgets and resource leases in the actual bootstrap. It requires
the request route, a registry without legacy `single_tab.*`, literal-loopback
runtime/HTTP addresses, a private runtime token, stable profile/host and explicit
canonical allowed origins. The resource authority is
`/internal/guarded-tab-resources`; runtime tickets stay private. Local selection
requests remain outside the tab lease route. Controlled-browser tools are hidden
in this initial single-profile catalog and deny direct admission. See
[operator setup and limits](../../docs/architecture/single-tab-production-composition.md).
Default remains disabled; local click completion leaves external effects unknown.

The separate default-false `BUTLER_GUARDED_TAB_REQUEST_ENABLED` registers only
`single_tab.bind.guarded`, an empty-args local control contract. It requires
PostgreSQL, private action authentication and fixed
`BUTLER_GUARDED_TAB_REQUEST_CONTROL_URL` ending `/internal/guarded-tab-request`.
Plain HTTP is limited to literal loopback; remote composition requires trusted
HTTPS. The exact contract, permanent claim and shared budget precede the owning
request RPC. No Chrome/legacy bind fallback or credential-bearing input exists.
Only an opaque request receipt returns, without candidates or native references.
Current owning readiness gates catalog/prepare; selection does not grant a
browser effect. This flag does not register guarded status/click runtime routes.

The optional converter developer route uses a distinct fixed reviewed
`task.delegate_converter` contract and private admission client. The initial
context-only `task.delegate` contract retains empty child permissions. A developer
child receives only discover/develop/test; `converters.collect` is a separate
parent action after current immutable final/report and tested-package proof.
Original trials/activation remain ordinary parent grants. Routing constructors
do not enable the feature or grant authority.

Optional resource admission also accepts the separate guarded user-tab adapter
when exact status/click contracts, current approved selection, finite shared
budget and complete source authority are composed. Tickets remain private gRPC
metadata. Successful local click ACK still leaves the external effect unknown.
Main guarded user-tab registration remains disabled; see the
[boundary and remaining gates](../../docs/architecture/single-tab-resource-boundary.md).

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
The optional delegation and converter-delegation Compose overlays supply only
these fixed routes/flags; use them with the existing converters overlay.

The global current-source data guard also repeats immutable055 continuation
authority before a tool claim. Already accepted `converter-return` inputs retain
this check after new developer admission is disabled. Ordinary inputs require no
055 record and gain no continuation authority. Core requires055 readiness before
advertising its composed developer route.

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

For owning converter continuations055, compose
`generatedtools.CheckContinuationRunTx` after the Catalog source guard for every
action. The owning check repeats exact queued input/fence/current authority and
complete original privacy dependencies; converter publication also repeats it.
An internal readiness input never creates owner permission, tool capability or
semantic completion. See the [contract](../../docs/architecture/converter-continuations.md).

Server browser viewing/manual control uses the private owner channel at
`/internal/server-browser` with existing core service authentication. Configure
`BUTLER_BROWSER_CONTROL_RUNTIME_URL` and its dedicated runtime token file via
`deploy/docker-compose.server-browser.yml`; migration086 is required. Broker
registers only schema-valid correlated successful opens, validates live ownership,
keeps durable takeover/release fences and content-free ACK audit, and blocks agent
calls before Claim while human control is active. An explicit fresh observation
is required on return. This composition supports ordinary workspace tools and is
separate from optional guarded shared user tabs. Standard `single_tab.*` contracts
are disabled. See the browser-session contract for limits and recovery.
Each session dispatch and fresh-observation update is pinned to its admitted
control epoch. A late old snapshot cannot unlock a new generation. Release of
the existing authenticated controller remains available after task cancellation
as cleanup; viewing and input still require live core authority.
## Isolated communication composition (#130)

The [communication lab command](cmd/communication-lab/README.md) adds canonical
message/photo/report contracts and the existing exact-dispatch executor to the
normal Broker on a disposable `_test` database. Normal `New`/main and production
configuration remain unchanged. Static registry files cannot define this family.

## Explicit owner rollout

`tool-broker -communication-profile /private/profile.json` registers only the
qualified message/photo/report private contracts through the existing dispatch
and authority checks. The strict v1 operator profile must match the exact current
database and configured owner, and pin the Core control URL; Core uses the same
profile. Without the flag legacy composition remains unchanged. The separate
lab constructor still requires `_test`. A profile never grants tool/task/source
authority or enables widgets, and cannot provide raw credential material.

### Widget composition

Private owner profile `Widgets=true` registers `communication.widget` alongside
the three accepted methods. Isolated communication-lab accepts `-widgets`; its
existing disposable database guard remains mandatory. Registration is not grant
authority: current immutable admission, target hash, exact prepare authorization,
source policy and finite budgets still govern dispatch.
