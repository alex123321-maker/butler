# Orchestrator

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

The opt-in `antigravity` provider uses native Go AICode v1 HTTP/SSE for Gemini 3.8 Flash and
normalizes native function proposals into the Orchestrator/Tool Broker flow.
Control-plane authorization owns encrypted PostgreSQL OAuth import and refresh;
transport reads no credential files and launches no CLI. The settings UI reports
stored credentials without claiming model quota. [Operator setup and validation
status](../../deploy/antigravity/README.md) document the private import and real
text/function/continuation gate. Current Google generation returns HTTP 429;
activation remains pending that gate.

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

Only typed kernel/operator/current owner policies are system instructions. Retrieved content, full transcript history, summary, task scratch and child observations remain data with source references; recovered known tool results do not become native orphan function responses. Provenance protection wraps the budgeted native provider. Production auxiliary extraction/summary/embedding jobs currently fail closed because they lack durable job-owned budget admission; profile/working/covered-summary reads remain available. Prompt preview/update and settings/provider mutations require owner proof. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).
Owner-defined initiative sources and schedules have durable normalized event
ingress and a bounded pull digest at `/api/v3/initiatives`. Maintenance coalesces
missed ticks and atomically admits ordinary tasks only under current owner
authority and execution budgets. The built-in heartbeat is an empty check;
fixed external observers and verified project-fix acceptance are still pending.
See [initiative admission](../../docs/architecture/initiative-contract.md).
