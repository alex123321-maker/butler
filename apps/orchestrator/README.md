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
- exposes an internal delivery sink for `assistant_delta` and `assistant_final` events without allowing channel adapters to mutate run state
- exposes `SubmitEvent` over gRPC and synchronous `POST /api/v1/events` over REST for normalized event ingestion
- exposes `GET /health` and `GET /metrics`

Dependencies:
- PostgreSQL for durable sessions, runs, and transcript state
- Redis for session leases and transient ownership state
- generated gRPC bindings from `proto/`

Configuration:
- required: `BUTLER_POSTGRES_URL`, `BUTLER_REDIS_URL`
- commonly used: `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, `BUTLER_LOG_LEVEL`, `BUTLER_SESSION_LEASE_TTL_SECONDS`, `BUTLER_OPENAI_MODEL`
- see `internal/config/config.go` and `.env.example` for the current typed config surface

Entry points and APIs:
- binary entrypoint: `apps/orchestrator/main.go`
- gRPC API: `SessionService` from `proto/session/v1/session.proto`
- gRPC API: `OrchestratorService` from `proto/orchestrator/v1/orchestrator.proto`
- HTTP endpoints: `/health`, `/metrics`, `/api/v1/events`
- internal execution package: `apps/orchestrator/internal/orchestrator`
- internal delivery seam: `apps/orchestrator/internal/orchestrator/delivery.go`

Local run:
- copy `.env.example` to `.env`
- start infra with `make infra-up`
- start the service with `go run ./apps/orchestrator`

Testing:
- unit and integration tests: `go test ./apps/orchestrator/...`
- smoke verification: `go run ./scripts/smoke/sprint2_event_flow.go`
- full repo checks: `go test ./...`, `go build ./...`, `go vet ./...`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/model-transport-contract.md`
- `docs/architecture/memory-model.md`
- `docs/testing/sprint-2-smoke.md`

Current limitations:
- the service executes the OpenAI transport path synchronously inside request handling, so REST ingestion returns only after run completion
- the current OpenAI transport backend is HTTP SSE only; WebSocket-first OpenAI transport is still pending
- tool-calling and resume paths are not wired into the service API yet
- Dockerfile exists, but the full service stack is not wired into Compose yet
