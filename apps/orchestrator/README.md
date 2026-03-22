# Orchestrator

Butler core API and orchestration service.

Current baseline:
- loads typed config from env through `internal/config`
- initializes shared structured logging
- connects to PostgreSQL and Redis on startup
- serves the Session Service gRPC contract on `BUTLER_GRPC_ADDR`
- implements `CreateSession`, `GetSession`, and `ResolveSessionKey` inside the orchestrator process
- implements Redis-backed `AcquireLease`, `RenewLease`, and `ReleaseLease`
- implements durable run creation, lookup, state transitions, and input-event deduplication
- implements the internal event-to-run execution flow for normalized `InputEvent` values using the transport layer and transcript store
- loads durable Working Memory snapshots into the run memory bundle and applies explicit save/update/clear policy across prepare, tool checkpoints, and finalize paths
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
- reuses the most recent same-provider `provider_session_ref` for a session on new runs when available, so immediate follow-up turns can continue provider-side state before async summaries catch up
- exposes an internal delivery sink for `assistant_delta` and `assistant_final` events without allowing channel adapters to mutate run state
- exposes `SubmitEvent` over gRPC and synchronous `POST /api/v1/events` over REST for normalized event ingestion
- optionally runs the in-process Telegram adapter using Bot API long polling, `/auth` provider connection prompts, and final-response delivery
- progressively edits Telegram responses during assistant delta streaming and finalizes the same message on completion
- exposes REST views for sessions, runs, transcripts, and doctor reports
- exposes grouped settings management endpoints for layered config overrides, masked secret display, restart planning, and helper-backed restart scheduling
- exposes provider auth endpoints for GitHub Copilot device flow and OpenAI Codex OAuth completion
- supports approval-gated tool execution with Telegram callback actions and formatted inline allow/deny prompts
- exposes `GET /health` and `GET /metrics`

Dependencies:
- PostgreSQL for durable sessions, runs, and transcript state
- Redis for session leases and transient ownership state
- generated gRPC bindings from `proto/`
- Tool Broker gRPC service for tool execution and runtime routing

Configuration:
- required: `BUTLER_POSTGRES_URL`, `BUTLER_REDIS_URL`
- commonly used: `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, `BUTLER_LOG_LEVEL`, `BUTLER_MODEL_PROVIDER`, `BUTLER_SESSION_LEASE_TTL_SECONDS`, `BUTLER_OPENAI_MODEL`, `BUTLER_OPENAI_CODEX_MODEL`, `BUTLER_GITHUB_COPILOT_MODEL`, `BUTLER_OPENAI_REALTIME_URL`, `BUTLER_OPENAI_TRANSPORT_MODE`, `BUTLER_TOOL_BROKER_ADDR`
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
- HTTP endpoints: `/health`, `/metrics`, `/api/v1/events`, `/api/v1/settings`, `/api/v1/settings/{key}`, `/api/v1/settings/restart`, `/api/v1/prompts/system`, `/api/v1/prompts/system/preview`, `/api/v1/providers`, `/api/v1/providers/{provider}/auth`, `/api/v1/providers/{provider}/auth/start`, `/api/v1/providers/{provider}/auth/complete`, `/api/v1/memory`, `/api/v1/sessions`, `/api/v1/sessions/{key}`, `/api/v1/runs/{id}`, `/api/v1/runs/{id}/transcript`, `/api/v1/doctor/reports`, `/api/v1/doctor/check`, `/api/v2/overview`, `/api/v2/tasks`, `/api/v2/tasks/{id}`, `/api/v2/approvals`, `/api/v2/approvals/{id}`, `/api/v2/approvals/{id}/approve`, `/api/v2/approvals/{id}/reject`, plus compatibility aliases `/api/overview`, `/api/tasks`, `/api/tasks/{id}`, `/api/approvals`, `/api/approvals/{id}`, `/api/approvals/{id}/approve`, `/api/approvals/{id}/reject`.
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

Current limitations:
- the service executes the active model transport path synchronously inside request handling, so REST ingestion returns only after run completion
- the OpenAI API path prefers Realtime WebSocket and falls back to HTTP SSE when the WebSocket backend is unavailable early in the run; GitHub Copilot and OpenAI Codex currently use HTTP streaming only
- tool calling now goes through Tool Broker and runtime services over gRPC
- some dashboard cards are still intentionally placeholder content, but the `/memory` view now browses durable memory records through the orchestrator API

Task-centric API v2 (Wave 1 start):
- `GET /api/v2/tasks` returns normalized task rows derived from run/session/message data without changing execution core ownership.
- `GET /api/v2/tasks/{id}` returns an aggregated task detail payload with `task`, `summary_bar`, `source`, `waiting_state`, `result`, `error`, `timeline_preview`, and `debug_refs`.
- `GET /api/v2/overview` returns aggregated `attention_items`, `active_tasks`, `recent_results`, `system_summary`, and `counts` so Web UI does not compose business overview from many client-side calls.
- `GET /api/tasks` is a compatibility alias with identical response shape.
- `GET /api/tasks/{id}` is a compatibility alias for the detail response.
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
  - resolve actions are idempotent and return `changed=false` with HTTP `409` if the approval is already terminal.

Artifacts baseline (Wave 2 continuation):
- Durable `artifacts` table stores user-meaningful outputs (`assistant_final`, `doctor_report`, `tool_result`, `summary`).
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
- `GET /api/v2/tasks/{id}/debug` (`/api/tasks/{id}/debug` alias) returns low-level run + transcript + tool call payloads for operator/debug mode.
- Supported filters: `status`, `needs_user_action`, `waiting_reason`, `source_channel`, `provider`, `from`, `to`, `query`, `limit`, `offset`, `sort`.
- Supported `sort` values: `started_at_desc` (default), `started_at_asc`, `updated_at_desc`, `updated_at_asc`.
- UI status mapping is explicit and run-derived:
  - active run states -> `in_progress`
  - `awaiting_approval` + Telegram source -> `waiting_for_reply_in_telegram`
  - `awaiting_approval` + non-Telegram source -> `waiting_for_approval`
  - `completed|failed|cancelled|timed_out` -> `completed|failed|cancelled|completed_with_issues`
