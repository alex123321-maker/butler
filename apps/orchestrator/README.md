# Orchestrator

Optional guarded tab owner decisions use `POST /api/v3/tab-choices/{approval_id}/select`
with only `candidate_token`, or `/revoke` with `{}`. Bearer owner proof is
mandatory; claimed actor/host/tab/lease fields cannot supply authority. Existing
finite waiting requests resolve their current binding from PostgreSQL. Selection
does not resume an executor or confirm a browser effect. The operator-only
`BUTLER_GUARDED_TAB_OWNER_ENABLED` defaults false; enabling requires fixed
`BUTLER_GUARDED_TAB_HOST`, `BUTLER_GUARDED_TAB_METADATA_URL`, private
`BUTLER_GUARDED_TAB_TOKEN_FILE` and configured owner authentication. Remote
metadata requires HTTPS; plain HTTP is limited to literal loopback addresses.
Startup checks migration063/064 columns and the exact enabled immutable trigger
function. Authenticated `GET /api/v3/tab-choices/{approval_id}` exposes only frozen
origin, candidate token, original ordinal, active-at-request marker and selection
state. Private native host/tab references remain excluded. The Approvals page
uses this view for guarded select/revoke and keeps owner proof only in page memory.
This endpoint does not enable guarded tools or declare runtime readiness. Worker
request creation, channel delivery/resumption and private Broker/runtime
composition are separate activation requirements. Older requests without frozen
064 display metadata have no reconstructed choice view.

Authenticated task observations at `/api/v3/tasks/{task_id}/observations` expose
owned assignment/initiative metadata, child result status, recorded action
consequences and shared root budget. Known token subtotals and unavailable usage
remain separate from conservative reservations. Run status never supplies an
action risk assessment. See the
[owner control contract](../../docs/architecture/task-owner-controls-spec.md).

Provider failures persist their fixed transport subtype in the failure message
with the existing `transport_error` class. Source verification distinguishes
stale sources from its own timeout. No provider text/details or unknown type
string is logged or returned by this failure path; retries remain disabled.

Optional generated-converter delegation053 has a distinct private admission
component. `NewDelegationLifecycleManager` selects immutable061 context-only or053
developer identity inside the owning transaction; it cannot admit new children and
does not retry another role after denial. Developer rights are only fixed
discover/develop/test, while original-data trials and activation stay parent actions.
Its report creator atomically retains the content-free054 tested return with the
actual immutable child final/report. `converters.collect` remains a distinct
parent Broker action. Provider/recovery/final guards include current return
privacy and policy, without turning technical preparation into semantic Accept.
No constructor enables routing or deployment. See
[the bounded converter contract](../../docs/architecture/generated-converters.md).
The isolated converter lab helper exercises this composition through ordinary
durable input workers, production Broker and a network-none actual runtime with
the app race detector. The earlier proof used explicit owner inputs; the055
process proof now covers two automatic same-task continuations. Native model
planning and final deployed acceptance remain separate gates.

The optional `singletab.ResourceGuard` locks the actual approved selection for
the separately guarded native-host status/click contracts. It requires prior
current worker/owner/task/source authority on the same transaction and the
immutable guarded request/selection marker; legacy approval/session activation
cannot create this proof. The owning `internal/singletabcontrol` library performs
atomic owner selection, with metadata RPC outside transaction locks. Its tested
composition requires a live worker lease; suspended human waiting/resume is a
separate pending authority adapter. No bootstrap/registry route is enabled by it;
see [guarded user-tab contract](../../docs/architecture/single-tab-resource-boundary.md).

The bounded [delegation manager](internal/task/README.md) provides transactional
child admission, dependency scheduling, durable result/report metadata and explicit
guarded decisions. It is an unenabled component until the owning Catalog/Broker
adapters and isolated child context are wired and verified. It does not turn a
model report or manager acceptance into task completion.

Ordinary delegated workers use the durable accepted input ID for run
deduplication. Core preparation/recovery accepts that private key mapping only
through the current input store's typed claim/fence verifier, while retaining the
original kernel event and exact versioned contract source. Missing verification,
stale claims and substituted keys fail before inference. Strict direct-core
events keep their original event key. Current Catalog selection and task/owner
authority remain independent checks.

[Reviewed skills](../../docs/architecture/reviewed-skills.md) use descriptions
before selected procedural DATA and reload current pins during recovery.
The private evidence API reads owned task/result/effect/transcript/budget APIs;
model verdicts never activate skills. The optional authenticated `/skills` page
relays owner decisions to the fixed private `BUTLER_SKILL_CONTROL_URL` endpoint.
Source access/deletion guards run before skill lifecycle and dependency guards
through inference, dispatch and final-result admission. See the
[actual acceptance and limits](../../docs/architecture/reviewed-skills-acceptance.md).

Tool execution prepares the exact action through Broker, records required
owner approval, then obtains a private dispatch grant tied to current task and
worker versions. `/internal/action-context` validates PostgreSQL ownership and
delegates durable approval checks. Configure `BUTLER_ACTION_AUTHORITY_TOKEN_FILE`,
`BUTLER_AUTONOMY_MODE` (default `mode_1`) and, when explicitly assigned,
`BUTLER_ACTION_MANDATE_ID`. Unknown effects stop before model continuation. See
[action authorization](../../docs/architecture/action-authorization-contract.md).

Browser guidance matches the explicit
[`browser.*` session contract](../../docs/architecture/browser-session-contract.md):
independent reads use `browser.read`, sequences begin with `browser.open`, follow-up
effects carry the current observation/URL, and lost state is reported explicitly.
The separately approved `single_tab.*` flow retains its own capability boundary.

The separate issue16 native acceptance fixture carries all four immutable failed
windows (13 attempted calls) into one newly owner-directed window: at most12 new
attempts,25 overall. Its durable counter and global baseline13 marker prevent
replay/reset; no failed provider call triggers retry or fallback. See the
[experiment plan](../../docs/ai/delegation-native-scenario-plan.md) for evidence.

When delegation is configured, authenticated owner controls at
`POST /api/v3/delegation-decisions/{parent_task_id}/{child_task_id}` accept
`kind: reject` with the exact collected `report_source` and `evidence_refs`, or
`kind: accept_verified` with the same pins plus `verified_result_id`. The latter
must reference a separately persisted owner verification for that same final
run. It preserves the original collected report/result and atomically generates
and accepts the checked report. Decisions do not complete tasks or resume work.
Without configured delegation, this route returns unavailable.

The opt-in `antigravity` provider uses native Go AICode v1 HTTP/SSE for Gemini 3.8 Flash and
normalizes native function proposals into the Orchestrator/Tool Broker flow.
Control-plane authorization owns encrypted PostgreSQL OAuth import and refresh;
transport reads no credential files and launches no CLI. The settings UI reports
stored credentials without claiming model quota. [Operator setup and validation
status](../../deploy/antigravity/README.md) document the private import and real
text/function/continuation gate. The lab's [native verification](../../deploy/execution-budgets/native-verification.json)
records actual direct Gemini 3.8 Flash inference, one owner-approved Broker HTTP
call and native tool-result continuation. Stored credentials alone still do not
prove model access or future backend capacity.

Tool execution fails closed when the approval policy cannot be checked. A tool
requiring approval also requires a configured approval gate. These guarantees
apply to single calls and batch members; policy failures do not become permission.
Approval waiters are registered before publication and notification, accept one
decision, and reject decisions after cancellation. A durable denial overrides an
in-memory approval. Restart recovery and version-bound authorization remain
tracked in issues #5–#7; an in-memory waiter is not durable execution state.

Butler core API and orchestration service.

Current baseline:
- loads typed config from env through `internal/config`
- initializes shared structured logging
- connects to PostgreSQL and Redis on startup
- serves the Session Service gRPC contract on `BUTLER_GRPC_ADDR`
- implements `CreateSession`, `GetSession`, and `ResolveSessionKey` inside the orchestrator process
- implements Redis-backed `AcquireLease`, `RenewLease`, and `ReleaseLease`
- cancels and joins the lease renewal worker before releasing ownership; in-flight renewal follows run cancellation and release has a bounded timeout
- synchronizes live event publication with subscriber removal and cleanup, preventing sends to closed SSE channels while keeping delivery nonblocking
- implements durable run creation, lookup, state transitions, and input-event deduplication
- implements the internal event-to-run execution flow for normalized `InputEvent` values using the transport layer and transcript store
- owns durable tasks separately from attempts; explicit task references load task-owned goal, criteria, constraints, plan and pending actions from PostgreSQL
- model final responses save unverified task results and never clear unfinished task work or mark the task completed
- stores transient Working Memory scratch state in Redis with TTL-based cleanup separate from durable PostgreSQL snapshots
- requests memory bundles from `internal/memory/service`, which owns scope ordering plus profile, episodic, working, and session-summary retrieval during run preparation
- writes durable working-memory provenance metadata tied to the active run and supports provenance-safe memory links for downstream profile/episodic records
- sanitizes credential-like values before memory extraction and before durable working-memory persistence so memory stores do not retain raw tokens, passwords, cookies, or DSNs
- runs an explicit async memory pipeline with extraction, classification, conflict resolution, and ignore/noise handling before profile and episodic persistence; in Codex-only setups extraction can run from provider auth while embedding-backed episodic/chunk writes remain disabled until an OpenAI-compatible API key is configured
- applies deterministic profile supersession and episodic duplicate/variant policy to reduce memory drift across repeated runs
- assembles hybrid memory bundles with ordered structured facts, episodic retrieval, optional keyword matches, and explicit prompt-budget trimming
- persists and retrieves reusable document chunks, including controlled doctor/tool-output derived chunks, through PostgreSQL-backed memory chunk storage
- relies on explicit housekeeping/retention policies for stale working snapshots, inactive profile versions, noisy episodic growth, and chunk overgrowth
- exposes memory-specific metrics and doctor diagnostics for pipeline jobs, queue depth, retrieval activity, and pgvector readiness
- exposes `GET /api/v1/memory` for scope-based browsing of durable working, profile, episodic, and chunk memory plus provenance-safe links
- enqueues async post-run memory extraction work and stores session summaries for later context reuse
- skips episodic similarity retrieval unless a real query-embedding provider is configured, rather than emitting placeholder vectors
- stores an operator-managed base system prompt in `system_settings` and assembles the effective prompt inside orchestrator during `preparing`
- supports allowlisted prompt placeholders for safe runtime section injection, adds an informational tool capability summary, and provides a preview endpoint for effective prompt inspection
- injects a built-in browser strategy section that survives operator prompt overrides and teaches browser-oriented follow-ups such as interpreting short numeric replies against the agent's most recent numbered browser options
- starts each new run with fresh provider state and Butler-owned context, preventing duplicate replay into an existing provider history; tool continuation within a run retains its own provider ref
- loads a source-versioned session summary and the complete uncompressed user/assistant tail after its verified message boundary; unknown coverage replays full history
- preserves durable task constraints separately from summaries; stops model preparation explicitly on history/coverage read errors or mandatory context overflow (constraint amendments remain in #9)
- admits context with `BUTLER_CONTEXT_BUDGET_TOKENS` (default 65536), using a conservative encoded UTF-8 byte estimate plus protocol allowance, then removes chunks, episodes, profile and descriptive tool summaries before rejecting oversized mandatory context; this estimate is not provider billing or a native tokenizer count
- exposes an internal delivery sink for `assistant_delta` and `assistant_final` events without allowing channel adapters to mutate run state
- exposes durable acceptance through gRPC `SubmitEvent` and HTTP 202 `POST /api/v1/events`; `/api/v3/inputs/{id}` observes asynchronous execution
- optionally runs the in-process Telegram adapter using Bot API long polling, `/auth` provider connection prompts, and final-response delivery
- sends best-effort partial Telegram drafts and delivers saved final notifications through the PostgreSQL outbox with per-chunk acknowledgement and explicit unknown outcomes
- exposes REST views for sessions, runs, transcripts, and doctor reports
- exposes grouped settings management endpoints for layered config overrides, masked secret display, restart planning, and helper-backed restart scheduling
- exposes provider auth endpoints for GitHub Copilot device flow and OpenAI Codex OAuth completion
- supports approval-gated tool execution with Telegram callback actions and formatted inline allow/deny prompts
- supports typed approval payloads, including browser tab selection with Web UI and Telegram delivery
- exposes single-tab bind orchestration endpoints for creating bind requests, reading tab-bound sessions, updating session state, and releasing them by `session_id`
- exposes extension-auth protected single-tab endpoints for remote browser-extension bind/session checks and bind-discovery relay polling
- enforces browser-instance ownership on remote extension relay dispatch polling and result callbacks
- marks active remote single-tab sessions as `HOST_DISCONNECTED` when extension heartbeat is stale before dispatch
- exposes `GET /api/v2/single-tab/extension-instances` for lightweight per-instance relay health polling
- exposes `GET /health` and `GET /metrics`

Dependencies:
- PostgreSQL for durable sessions, runs, and transcript state
- PostgreSQL for authoritative ownership fencing, results, final-notification outbox and auxiliary jobs; Redis for transient working state and legacy memory operator jobs
- generated gRPC bindings from `proto/`
- Tool Broker gRPC service for tool execution and runtime routing

Configuration:
- required: `BUTLER_POSTGRES_URL`, `BUTLER_REDIS_URL`
- commonly used: `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, `BUTLER_LOG_LEVEL`, `BUTLER_MODEL_PROVIDER`, `BUTLER_SESSION_LEASE_TTL_SECONDS`, `BUTLER_OPENAI_MODEL`, `BUTLER_OPENAI_CODEX_MODEL`, `BUTLER_GITHUB_COPILOT_MODEL`, `BUTLER_OPENAI_REALTIME_URL`, `BUTLER_OPENAI_TRANSPORT_MODE`, `BUTLER_TOOL_BROKER_ADDR`
- remote extension auth: `BUTLER_EXTENSION_API_TOKENS` enables `Authorization: Bearer` checks for `/api/v2/extension/single-tab/*`
- single-tab transport rollout: `BUTLER_SINGLE_TAB_TRANSPORT_MODE` supports `native_only`, `dual`, `remote_preferred` (default `dual`)
- relay liveness: `BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS` sets stale heartbeat threshold before dispatch marks session `HOST_DISCONNECTED`
- restart integration: `BUTLER_RESTART_HELPER_URL` points to the internal restart-helper control-plane service
- memory loading: `BUTLER_MEMORY_PROFILE_LIMIT`, `BUTLER_MEMORY_EPISODIC_LIMIT`, `BUTLER_MEMORY_SCOPE_ORDER`
- memory loading: `BUTLER_MEMORY_PROFILE_LIMIT`, `BUTLER_MEMORY_EPISODIC_LIMIT`, `BUTLER_MEMORY_SCOPE_ORDER`, `BUTLER_MEMORY_WORKING_TRANSIENT_TTL_SECONDS`
- Telegram adapter: `BUTLER_TELEGRAM_BOT_TOKEN`, `BUTLER_TELEGRAM_ALLOWED_CHAT_IDS`, `BUTLER_TELEGRAM_BASE_URL`, `BUTLER_TELEGRAM_POLL_TIMEOUT_SECONDS`
- settings overrides: `BUTLER_SETTINGS_ENCRYPTION_KEY` enables AES-GCM encryption for secret values stored in `system_settings`
- prompt management: `BUTLER_BASE_SYSTEM_PROMPT`, `BUTLER_BASE_SYSTEM_PROMPT_ENABLED`
- see `internal/config/config.go` and `.env.example` for the current typed config surface

Settings behavior:
- effective values resolve in this order: environment variables, database overrides, then code defaults
- blank environment variables are treated as unset, so empty Compose pass-through values do not override database or default values
- hot settings are applied to the in-process hot config container immediately; cold settings are stored and returned with `requires_restart=true`
- secrets remain masked in API responses; passwords and connection strings are fully masked, API keys show the last four characters, and Telegram bot tokens keep the first six plus last three characters
- prompt edits apply on the next run without restart; empty or disabled operator prompts fall back to Butler's built-in safe default base prompt

Entry points and APIs:
- binary entrypoint: `apps/orchestrator/main.go`
- gRPC API: `SessionService` from `proto/session/v1/session.proto`
- gRPC API: `OrchestratorService` from `proto/orchestrator/v1/orchestrator.proto`
- HTTP endpoints: `/health`, `/metrics`, `/api/v1/events`, `/api/v1/settings`, `/api/v1/settings/{key}`, `/api/v1/settings/restart`, `/api/v1/prompts/system`, `/api/v1/prompts/system/preview`, `/api/v1/providers`, `/api/v1/providers/{provider}/auth`, `/api/v1/providers/{provider}/auth/start`, `/api/v1/providers/{provider}/auth/complete`, `/api/v1/memory`, `/api/v1/sessions`, `/api/v1/sessions/{key}`, `/api/v1/runs/{id}`, `/api/v1/runs/{id}/transcript`, `/api/v1/doctor/reports`, `/api/v1/doctor/check`, `/api/v2/overview`, `/api/v2/tasks`, `/api/v2/tasks/{id}`, `/api/v2/approvals`, `/api/v2/approvals/{id}`, `/api/v2/approvals/{id}/approve`, `/api/v2/approvals/{id}/reject`, `/api/v2/approvals/{id}/select-tab`, `/api/v2/single-tab/bind-requests`, `/api/v2/single-tab/session`, `/api/v2/single-tab/extension-instances`, `/api/v2/single-tab/session/{id}`, `/api/v2/single-tab/session/{id}/state`, `/api/v2/single-tab/session/{id}/release`, `/api/v2/single-tab/actions/dispatch`, `/api/v2/extension/single-tab/bind-requests`, `/api/v2/extension/single-tab/bind-requests/next`, `/api/v2/extension/single-tab/bind-requests/{dispatch_id}/result`, `/api/v2/extension/single-tab/session`, `/api/v2/extension/single-tab/actions/next`, `/api/v2/extension/single-tab/actions/{dispatch_id}/result`, plus compatibility aliases `/api/overview`, `/api/tasks`, `/api/tasks/{id}`, `/api/approvals`, `/api/approvals/{id}`, `/api/approvals/{id}/approve`, `/api/approvals/{id}/reject`, `/api/approvals/{id}/select-tab`, `/api/single-tab/bind-requests`, `/api/single-tab/session`, `/api/single-tab/extension-instances`, `/api/single-tab/session/{id}`, `/api/single-tab/session/{id}/state`, `/api/single-tab/session/{id}/release`.
- internal execution package: `apps/orchestrator/internal/orchestrator`
- internal delivery seam: `apps/orchestrator/internal/orchestrator/delivery.go`
- internal session/run boundary: `apps/orchestrator/internal/session` (gRPC server), `apps/orchestrator/internal/run` (run state machine and storage)
- domain↔proto enum conversions: `internal/domain/convert` (shared by session, run, and orchestrator packages)

Internal code map:
- app bootstrap is split across `internal/app/bootstrap.go`, `internal/app/bootstrap_http.go`, `internal/app/runtime.go`, and `internal/app/memory_adapters.go`
- run execution flow is split across `internal/orchestrator/service.go`, `internal/orchestrator/service_execute.go`, `internal/orchestrator/service_state.go`, and `internal/orchestrator/service_prepare.go`
- tool execution and working-memory persistence remain in `internal/orchestrator/tool_handler.go` and `internal/orchestrator/working_memory.go`
- HTTP endpoint groups stay under `internal/api/` with one file per endpoint family where practical

Local run:
- copy `.env.example` to `.env`
- start infra with `make infra-up`
- start the service with `go run ./apps/orchestrator`
- or build and start the Compose MVP stack with `make up`
- stop the Compose MVP stack with `make down`
- inspect unified service logs with `make logs`

Testing:
- unit and integration tests: `go test ./apps/orchestrator/...`
- smoke verification: `go run ./scripts/smoke/sprint2_event_flow.go`
- Telegram manual check: `docs/testing/telegram-manual.md` (`/auth` now opens provider selection and fallback prompts when auth is missing)
- Sprint 3 end-to-end check: `docs/testing/sprint-3-e2e.md`
- full repo checks: `go test ./...`, `go build ./...`, `go vet ./...`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/model-transport-contract.md`
- `docs/architecture/memory-model.md`
- `docs/architecture/prompt-management.md`
- `docs/testing/sprint-2-smoke.md`
- `docs/testing/telegram-manual.md`
- `docs/testing/sprint-3-e2e.md`
- `docs/testing/single-tab-browser-smoke.md`

Current limitations:
- the service executes the active model transport path synchronously inside request handling, so REST ingestion returns only after run completion
- the OpenAI API path prefers Realtime WebSocket and falls back to HTTP SSE when the WebSocket backend is unavailable early in the run; GitHub Copilot and OpenAI Codex currently use HTTP streaming only
- tool calling now goes through Tool Broker and runtime services over gRPC
- some dashboard cards are still intentionally placeholder content, but the `/memory` view now browses durable memory records through the orchestrator API

Task-centric API v2 (Wave 1 start):
- `GET /api/v2/tasks` reads durable task rows with independent `task_status`, `run_state`, `legacy` and `result_verified` fields. Conversation attempts remain in `/api/v1/runs`.
- `GET /api/v2/tasks/{id}` returns an aggregated task detail payload with `task`, `summary_bar`, `source`, `waiting_state`, `result`, `error`, `timeline_preview`, and `debug_refs`.
- `GET /api/v2/overview` returns aggregated `attention_items`, `active_tasks`, `recent_results`, `system_summary`, and `counts` so Web UI does not compose business overview from many client-side calls.
- `GET /api/tasks` is a compatibility alias with identical response shape.
- `GET /api/tasks/{id}` is a compatibility alias for the detail response.

Durable task owner controls use `/api/v3/tasks`, `/{id}/working`, `/{id}/status`
and `/{id}/results`; continuation uses explicit `payload.task_id` on the existing
event API. See `docs/architecture/durable-tasks-spec.md` for request fields,
verification, trusted API assumptions and migration semantics. Session working
memory remains a legacy audit view; current task work belongs to the task module.
- `GET /api/overview` is a compatibility alias for the overview response.

Durable approvals baseline (Wave 2 start):
- Approval flow now writes a durable pending record before waiting on in-memory approval gate.
- Telegram approval callbacks resolve both the in-memory waiter and durable approval status through a shared approval service.
- New migrations:
  - `022_approvals_and_approval_events` adds `approvals` and `approval_events` tables for durable approval state and audit trail.
- This preserves the same orchestration path while making approval state visible to task-centric Web UI and operator diagnostics.
- Approvals API (v2):
  - `GET /api/v2/approvals`
  - `GET /api/v2/approvals/{id}`
  - `POST /api/v2/approvals/{id}/approve`
  - `POST /api/v2/approvals/{id}/reject`
  - `POST /api/v2/approvals/{id}/select-tab`
  - resolve actions are idempotent and return `changed=false` with HTTP `409` if the approval is already terminal.
- Single-tab bind API:
  - `POST /api/v2/single-tab/bind-requests` creates a durable `browser_tab_selection` approval and fan-outs delivery to Web UI and Telegram.
  - `POST /api/v2/single-tab/bind-requests` also supports extension-driven candidate discovery with `discover_tabs_via_extension=true` when tab candidates are not provided directly.
  - `GET /api/v2/single-tab/session?session_key=...` returns the active tab-bound session for the Butler session when present.
  - `GET /api/v2/single-tab/session/{id}` returns one durable tab-bound session by `single_tab_session_id`.
  - `POST /api/v2/single-tab/session/{id}/state` updates the current tab-derived status, URL, title, and heartbeat metadata.
  - `POST /api/v2/single-tab/session/{id}/release` marks an active session as `REVOKED_BY_USER` and records the release in approval audit events.
  - extension bind relay:
    - `GET /api/v2/extension/single-tab/bind-requests/next`
    - `POST /api/v2/extension/single-tab/bind-requests/{dispatch_id}/result`
- Browser capture artifact API:
  - `POST /api/v2/artifacts/browser-captures` stores a durable `browser_capture` artifact used by `single_tab.capture_visible`.

Artifacts baseline (Wave 2 continuation):
- Durable `artifacts` table stores user-meaningful outputs (`assistant_final`, `doctor_report`, `tool_result`, `summary`).
- Durable `browser_capture` artifacts can now store single-tab screenshots as artifact-backed image references instead of inline tool payloads.
- Artifact creation hooks are attached to:
  - assistant final persistence in orchestrator finalization,
  - completed tool calls,
  - doctor check reports.
- Artifacts API (v2):
  - `GET /api/v2/artifacts`
  - `GET /api/v2/artifacts/{id}`
  - `GET /api/v2/tasks/{id}/artifacts`

Task activity baseline:
- Added durable `task_activity` persistence for process-level timeline events distinct from transcript.
- Orchestrator observability events are mirrored into `task_activity` via activity service mapping.
- Activity APIs:
  - `GET /api/v2/tasks/{id}/activity`
  - `GET /api/v2/activity`

Channel delivery visibility baseline:
- Added durable `channel_delivery_events` persistence for explicit channel delivery state.
- Delivery observer records Telegram/Web delivery outcomes for assistant deltas/finals, approval requests, tool-call notifications, and status events.
- Task detail and overview mapping can now surface `waiting_for_reply_in_telegram` using delivery facts, not only run-state inference.

System and debug read APIs:
- `GET /api/v2/system` (`/api/system` alias) returns operator summary with `health`, `doctor`, `providers`, `queues`, `pending_approvals`, `recent_failures`, `degraded_components`, and `partial_errors`.
- system summary now includes `single_tab_extension` with rollout mode, relay heartbeat TTL, extension-auth readiness, and active/disconnected session counters for remote extension operations.
- `GET /api/v2/single-tab/extension-instances` (`/api/single-tab/extension-instances` alias) returns lightweight per-browser extension instance health with state filtering (`state=online,stale,...`) and explicit liveness policy metadata for UI polling.
- `GET /api/v2/tasks/{id}/debug` (`/api/tasks/{id}/debug` alias) returns low-level run + transcript + tool call payloads for operator/debug mode.
- Supported filters: `status`, `needs_user_action`, `waiting_reason`, `source_channel`, `provider`, `from`, `to`, `query`, `limit`, `offset`, `sort`.
- Supported `sort` values: `started_at_desc` (default), `started_at_asc`, `updated_at_desc`, `updated_at_asc`.
- UI status mapping is explicit and run-derived:
  - active run states -> `in_progress`
  - `awaiting_approval` + Telegram source -> `waiting_for_reply_in_telegram`
  - `awaiting_approval` + non-Telegram source -> `waiting_for_approval`
  - `completed|failed|cancelled|timed_out` -> `completed|failed|cancelled|completed_with_issues`

## Durable execution and owner decisions

Production ingress persists acceptance before returning and executes in the app
worker context. PostgreSQL provides ordered claims and expiring fenced session
ownership. Context preparation follows ownership. Durable approval continuations
release the worker and resume the same run/action after an authenticated owner
reply; a missing or uncertain external result blocks recovery without replay.
Suspension discards private provider context. Fresh recovery replays every known
tool observation as data and preserves exact native signatures only for new live
tool continuations; it never reconstructs signatures from transcript evidence.
See [the contract](../../docs/architecture/durable-execution-spec.md) for deadlines,
checkpoint boundaries, migration policy and the explicit delivery limitations.

Web decisions require `BUTLER_OWNER_CONTROL_OWNER_ID` plus an operator-provisioned
`BUTLER_OWNER_CONTROL_TOKEN` or `BUTLER_OWNER_CONTROL_TOKEN_FILE` (at least 32 bytes).
Without this authority approve/reject deny access. Generate/read the token on the
lab terminal; never place it in model context or chat. The configured owner must
match the saved web session owner. Telegram uses the allowed chat and numeric owner.

Before preparing a tool proposal, the durable input checkpoint repository
allocates a random logical action slot with pinned task/constraints versions.
Approval continuation and recovery preserve this system reference; it never
comes from model args. Effect IDs/states/evidence references accompany dispatch
and result checkpoints. Generic write completion remains uncertain; recovery
does not repeat it. See [the effect journal](../../docs/architecture/external-effect-journal.md).

Finalization requires migration `034_atomic_results_outbox`. One fenced transaction
saves the required assistant artifact, final transcript, immutable attempt result,
unverified task result, run completion, notification chunks and auxiliary jobs.
A model final never marks user work verified. Delivery errors and memory failures
cannot undo a committed result; finalizing recovery retries only saved-answer
persistence. `GET /api/v2/results/{run_id}` exposes final result, delivery and
auxiliary job states; task detail/overview prefer durable outbox observations.
See [atomic result and delivery](../../docs/architecture/atomic-result-delivery.md).

Task mutations also require owner-control authentication. `/api/v3/tasks/{id}/control` applies a revisioned amendment, pause, resume or tree stop immediately, revoking old worker authority and preserving earlier observations. See [Owner Controls](../../docs/architecture/task-owner-controls-spec.md).

Production inference and child admission now use migration037's shared root,
assigned mandate and owner UTC-day budgets. Configure the same deployment-only
`BUTLER_EXECUTION_BUDGET_POLICY_JSON` in Orchestrator and Broker; an exact explicit
model cost basis is required. Exhaustion/loops persist a blocked continuation and
concrete reason, cancel transport and never create a verified result. Frozen
finalization does not reserve another model turn. Owner-proved
`GET /api/v3/execution-budgets` exposes conservative reservations separately from
actual usage. Native Antigravity currently declares inference limits; undeclared
adapters and all batches fail closed. See
[execution budgets](../../docs/architecture/execution-budget-contract.md).

Authenticated owner goals and broad autonomy are available at `/goals` and the
`/api/v3/owner-goals`, `/api/v3/owner-mandates`, `/api/v3/owner-authority` routes.
Goal/mandate amendments persist exact revisions, source and restartable branch
propagation jobs. Current assignment guards deny dispatch, model admission and
atomic results immediately after revocation even if propagation is pending.
The owner token stays outside model context and browser storage. Ordinary tasks
and conversation remain possible without goal assignments. See
[owner goals and mandates](../../docs/architecture/owner-goals-mandates-spec.md).

Owner-defined initiative sources and schedules have durable normalized event
ingress and a bounded owner view at `/api/v3/initiatives`. Maintenance coalesces
missed ticks and atomically admits ordinary tasks only under current owner
authority and execution budgets. An owner-configured PostgreSQL `task_project`
observer follows exact current project/task revisions, deduplicates distinct
attempts, suppresses own origin and recognizes only verified healthy results.
An optional session-matched digest channel groups count-only admitted/verified
changes with active-hours and daily limits; uncertain Telegram delivery is not
repeated. The unconfigured heartbeat is an empty check. The `broker_project`
observer additionally reads confirmed reviewed extension checks and rechecks
current Catalog source pins and the independent deletion anchor. Historical
observations never reuse old leases. Digest authorization repeats current source,
goal and mandate checks immediately before sending. Actual Broker/executor lab
acceptance verifies an admitted prepared patch and owner result; live generative
planning remains pending. See [initiative admission](../../docs/architecture/initiative-contract.md)
and [lab evidence](../../docs/architecture/initiative-acceptance.md).

The optional `/extensions` page and owner-only `/api/v4/extensions` forward typed
verified owner provenance to the private Broker control endpoint configured by
`BUTLER_EXTENSION_CONTROL_URL`. The channel token never reaches that endpoint.
Layered startup reads that private endpoint from the operator environment only;
database settings cannot redirect the service-authenticated control relay.
Task attempts are fenced and pinned before scoped tool discovery. Catalog errors
retain the attempt and block it before inference; they never claim completion.
Fresh approval recovery reloads permitted definitions and manifest DATA while
preserving recorded observations without redispatch. See the
[reviewed lifecycle](../../docs/architecture/reviewed-extension-lifecycle.md).
Only typed kernel/operator/current owner policies are system instructions. Retrieved content, full transcript history, summary, task scratch and child observations remain data with source references; recovered known tool results do not become native orphan function responses. Provenance protection wraps the budgeted native provider. Legacy auxiliary extraction/summary/embedding jobs fail closed because they lack durable job-owned budget admission; profile/working/covered-summary reads remain available. Separately owned local indexing and live query use the explicit composition below. Prompt preview/update and settings/provider mutations require owner proof. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).

Migrations 046–049 add authenticated `/api/v4/memory` inspection, candidates,
revisioned correction, exact supersession, export and recursive privacy deletion.
Provider release/final events and atomic results recheck prepared transcript/source
pins. The scoped restartable Redis cleanup worker never flushes unrelated keys.
Deploy only after provisioning the private persistent deletion anchor shared with
Broker; missing/wrong anchors fail closed. See [versioned memory sources](../../docs/architecture/versioned-memory-sources.md).

The private memory page now inspects revision/claim history, corrects, supersedes,
exports and deletes owner sources. Selected original files use bounded Go
structured parsers with a separately sandboxed optional PDF/OCR service. Failed
files retain diagnostics and publish no partial chunks. `/source`, `/original`,
`/ingest`, `/index` and `/index-review` keep owner proof/no-store headers and exact
source/claim versions. Owned local indexing is opt-in; its job context wraps every
read/publication and actual IPC, and live retrieval uses the current run token.
See the [indexing composition](../../internal/memory/indexing/README.md) and
[parser confinement](../memory-parser/README.md). Missing model token, fixed local
identity or declared local price disables only that owned semantic path; legacy
cloud memory model jobs remain denied.

## Delegation foundation

The internal delegation contract, tree admission/owner-control barrier, executor
manager and private Broker control adapter are implemented behind Task Service.
`BUTLER_DELEGATION_ENABLED` defaults false. Explicit operator opt-in composes
`/internal/delegation`, owned context transitions and bounded maintenance over
the existing durable input workers. `BUTLER_DELEGATION_MAX_PARALLEL` accepts
one or two, defaults one and cannot be overridden through database settings.
Private action-authority authentication and migrations057/061 are required.
Startup verifies the immutable schema guards before advertising readiness.
Disabling the feature prevents new admission/maintenance; use owner task/tree
stop or mandate revocation for already accepted durable child inputs.
Native model comparison and deployment acceptance remain pending. See
`docs/architecture/delegation-contract.md` before enabling the route.
The separate opt-in native lab runner and its twelve-attempt spending cap are
described in `docs/ai/delegation-native-scenario-plan.md`. Its zero-inference
preflight is distinct from coordinated actual model acceptance.

Ordinary preparation/recovery now derives selected delegated context from the
actual fenced run, after pinning its task. It skips ambient memory bundles and
adds only selected Catalog DATA plus the child's own session/task context.
Durable snapshots carry references and reload bodies under the new live lease;
candidate prose is never promoted to owner policy. Migration057 is required for
the owned context reader, even while model/owner delegation admission is disabled.

Finite shared-resource leases are implemented in `internal/resources` with live
worker/task/parent checks on the caller PostgreSQL transaction. Migration058 and
production workspace/browser composition remain in progress; resource leases
alone cannot authorize a tool effect. See `docs/architecture/resource-lease-contract.md`.

`app.DelegationGuards` now validates exact current capability subsets and compares
structured child reports against real final/artifact/transcript, action journal,
source graph and branch reservation records on the caller PG transaction. Its
capability reader is mandatory and phase-explicit: new admission needs a current
claimed delegate action, lifecycle needs that child's durable accepted snapshot
and current policy. The private adapter stores immutable accepted capabilities
on the same admission transaction; the model experiment remains pending.
The initial `task.delegate` admits children with an empty tool allowlist.
Active completed partial finals remain distinct from blocked/failed attempts.
With delegation enabled, the parent's ordinary preparation/recovery now reads
typed failed-attempt observations under its own current lease and source access.
Historical runs/actions retain their actual task revision even when the ordinary
failure outcome advances the blocked task revision. Changed child constraints
or any unfinished child run deny the read. They remain private SubagentOutput DATA with nullable actual usage, journal
uncertainty and no completed result. Checkpoints retain reload markers and fresh
reads preserve later reconciliation/deletion. Observation does not wake the
parent, accept a child or free execution width. See
`docs/ai/delegation-attempt-acceptance.md` and
`docs/ai/delegation-guard-acceptance.md`.

Trusted report projection and bounded maintenance now collect actual committed
finals atomically before advancing already admitted waiting branches. Generated
reports contain only owning source/result/journal/reservation references and
current uncertainty, with no model-authored success verdict. Replay keeps one
report source; a revoked original rolls publication back. The optional context
lifecycle adapter marks execution under the actual child fence before input
release. Operator opt-in composes these adapters with private Broker admission.
Maintenance combines independent branch errors without suppressing unrelated
initiative review; it never admits a new model proposal by itself.

Optional `/api/v4/converters` and `/converters` relay only owner-proof metadata
inspection and exact-version revocation to the fixed operator/environment private
Broker endpoint. Provider release/recovery and atomic completion compose current
source/task/owner checks before converter use/lifecycle guards; old observations
cannot revive a revoked digest/generation. Bodies remain Catalog DATA and only
SourceRefs/hashes enter saved actions or final-result summaries. Technical conversion
does not complete semantic task criteria. See the
[generated converter contract](../../docs/architecture/generated-converters.md).

`NewDelegationAdmission(pool, taskStore, sessionAdapter, inputAdapter,
catalogAdapter, limits)` now supplies the optional private Broker admission
component. Every adapter is mandatory; the Task store needs shared budget
wiring. `AdmitDispatched` derives parent ownership from the actual executing
action/grant/effect, repeats current source/task/owner policy in the manager
transaction and stores immutable migration061 capabilities during source binding
before queue/receipt. Initial children have no tools. The guard wrapper embeds
`DelegationGuards`, forwarding its optional report-creator capability when that
component is composed. Its public `Manager` is for trusted bounded maintenance,
not a caller-supplied worker authority. Accepted child lifecycle uses its own
snapshot/current policy after parent process completion; no old lease is revived.
The component alone does not enable a route. Bootstrap composes its private
handler and lifecycle adapters only under `BUTLER_DELEGATION_ENABLED`; current
action contexts advertise readiness only after this composition succeeds.
Actual separate-process Broker/HTTP/PG evidence and pending model/deployment
criteria are recorded in `docs/ai/delegation-admission-acceptance.md`.

`BUTLER_CONVERTER_DELEGATION_ENABLED`, default false, adds a separate fixed
`/internal/converter-delegation` private handler. It requires ordinary delegation,
the guarded converter control endpoint, the exact operator runtime profile and
immutable053/054/055 readiness. Both accepted roles use the same current lifecycle
manager and ordinary input workers; the context reader advertises the distinct
developer readiness only after this composition. Provider/recovery and atomic
completion repeat source, developer and technical-return guards before installed
converter guards. A child report or local acknowledgement never completes a task.
See [generated converters](../../docs/architecture/generated-converters.md).
Compose operators add the explicit delegation and converter-delegation overlays
alongside the existing converters overlay; the base stack keeps both roles off.


The optional owning055 continuation service atomically queues two once-only
technical phases on the original task after confirmed developer return and
actual parent activation. Ordinary workers/leases/Broker execute them; inputs
carry opaque references and current readiness stays DATA. Full original document
pins and current private claim/fence/authority repeat at provider/recovery/final.
Its compound source verifier has a static2s bound (ordinary checks200ms), with
uncached proposal/final/periodic checks. See
[composition and evidence](../../docs/architecture/converter-continuations.md).

Bootstrap composes this service under the existing converter-delegation flag,
advertises readiness only after immutable055 checks, installs its opaque input
reader, and joins continuation maintenance with existing lifecycle review.
Its prospective allowance requires finite declared provider limits that fit the
actual execution policy; a bookkeeping quote cannot reduce a request limit.
Atomic final guards retain055 checks even if later new admission is disabled.
This composition adds no default enablement or semantic completion authority.
Optional `/api/v3/voice/` composition uses `VoiceServer`, current catalog/owned jobs,
`VoiceInputAdapter` and `VoiceControlAdapter`. Registering it does not enable it.
The post-lease `InputSourcePreparation` hook resolves exact stored input bindings;
checkpoint/approval projection retains refs rather than copied ASR/selected DATA
prose. Source refs never grant their own permission. Reviewed Stop calls the common
owner controller with a same-transaction current-source guard, keeping tree fences
and exact task revision checks. Text Stop remains independent. The voice runtime,
caller and human quality evidence are documented in
[voice contract](../../docs/architecture/russian-voice.md); all profiles/caller
flags remain off because the fixed Russian quality gate failed.

Authenticated task observations include durable background queue metadata across
the owned root's branches. Requested model, persisted state, attempts, expired
deadline and obsolete task revision remain separate from runtime readiness and
verified task completion. Raw source/error/claim/allowance payloads are excluded;
the read-only projection grants no execution authority.
