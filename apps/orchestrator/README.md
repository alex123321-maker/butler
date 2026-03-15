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
- enqueues async post-run memory extraction work and stores session summaries for later context reuse
- skips episodic similarity retrieval unless a real query-embedding provider is configured, rather than emitting placeholder vectors
- exposes an internal delivery sink for `assistant_delta` and `assistant_final` events without allowing channel adapters to mutate run state
- exposes `SubmitEvent` over gRPC and synchronous `POST /api/v1/events` over REST for normalized event ingestion
- optionally runs the in-process Telegram adapter using Bot API long polling and final-response delivery
- progressively edits Telegram responses during assistant delta streaming and finalizes the same message on completion
- exposes REST views for sessions, runs, transcripts, and doctor reports
- exposes grouped settings management endpoints for layered config overrides, masked secret display, and restart planning
- exposes provider auth endpoints for GitHub Copilot device flow and OpenAI Codex OAuth completion
- supports approval-gated tool execution with Telegram callback actions
- exposes `GET /health` and `GET /metrics`

Dependencies:
- PostgreSQL for durable sessions, runs, and transcript state
- Redis for session leases and transient ownership state
- generated gRPC bindings from `proto/`
- Tool Broker gRPC service for tool execution and runtime routing

Configuration:
- required: `BUTLER_POSTGRES_URL`, `BUTLER_REDIS_URL`
- commonly used: `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, `BUTLER_LOG_LEVEL`, `BUTLER_MODEL_PROVIDER`, `BUTLER_SESSION_LEASE_TTL_SECONDS`, `BUTLER_OPENAI_MODEL`, `BUTLER_OPENAI_CODEX_MODEL`, `BUTLER_GITHUB_COPILOT_MODEL`, `BUTLER_OPENAI_REALTIME_URL`, `BUTLER_OPENAI_TRANSPORT_MODE`, `BUTLER_TOOL_BROKER_ADDR`
- memory loading: `BUTLER_MEMORY_PROFILE_LIMIT`, `BUTLER_MEMORY_EPISODIC_LIMIT`, `BUTLER_MEMORY_SCOPE_ORDER`
- memory loading: `BUTLER_MEMORY_PROFILE_LIMIT`, `BUTLER_MEMORY_EPISODIC_LIMIT`, `BUTLER_MEMORY_SCOPE_ORDER`, `BUTLER_MEMORY_WORKING_TRANSIENT_TTL_SECONDS`
- Telegram adapter: `BUTLER_TELEGRAM_BOT_TOKEN`, `BUTLER_TELEGRAM_ALLOWED_CHAT_IDS`, `BUTLER_TELEGRAM_BASE_URL`, `BUTLER_TELEGRAM_POLL_TIMEOUT_SECONDS`
- settings overrides: `BUTLER_SETTINGS_ENCRYPTION_KEY` enables AES-GCM encryption for secret values stored in `system_settings`
- see `internal/config/config.go` and `.env.example` for the current typed config surface

Settings behavior:
- effective values resolve in this order: environment variables, database overrides, then code defaults
- blank environment variables are treated as unset, so empty Compose pass-through values do not override database or default values
- hot settings are applied to the in-process hot config container immediately; cold settings are stored and returned with `requires_restart=true`
- secrets remain masked in API responses; passwords and connection strings are fully masked, API keys show the last four characters, and Telegram bot tokens keep the first six plus last three characters

Entry points and APIs:
- binary entrypoint: `apps/orchestrator/main.go`
- gRPC API: `SessionService` from `proto/session/v1/session.proto`
- gRPC API: `OrchestratorService` from `proto/orchestrator/v1/orchestrator.proto`
- HTTP endpoints: `/health`, `/metrics`, `/api/v1/events`, `/api/v1/settings`, `/api/v1/settings/{key}`, `/api/v1/settings/restart`, `/api/v1/providers`, `/api/v1/providers/{provider}/auth`, `/api/v1/providers/{provider}/auth/start`, `/api/v1/providers/{provider}/auth/complete`, `/api/v1/sessions`, `/api/v1/sessions/{key}`, `/api/v1/runs/{id}`, `/api/v1/runs/{id}/transcript`, `/api/v1/doctor/reports`, `/api/v1/doctor/check`
- internal execution package: `apps/orchestrator/internal/orchestrator`
- internal delivery seam: `apps/orchestrator/internal/orchestrator/delivery.go`

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
- Telegram manual check: `docs/testing/telegram-manual.md`
- Sprint 3 end-to-end check: `docs/testing/sprint-3-e2e.md`
- full repo checks: `go test ./...`, `go build ./...`, `go vet ./...`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/model-transport-contract.md`
- `docs/architecture/memory-model.md`
- `docs/testing/sprint-2-smoke.md`
- `docs/testing/telegram-manual.md`
- `docs/testing/sprint-3-e2e.md`

Current limitations:
- the service executes the active model transport path synchronously inside request handling, so REST ingestion returns only after run completion
- the OpenAI API path prefers Realtime WebSocket and falls back to HTTP SSE when the WebSocket backend is unavailable early in the run; GitHub Copilot and OpenAI Codex currently use HTTP streaming only
- tool calling now goes through Tool Broker and runtime services over gRPC
- the web dashboard still includes placeholder sections for memory browsing and some dashboard cards that are intentionally left for later work
