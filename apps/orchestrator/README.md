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
- exposes `GET /health` and `GET /metrics`
- exposes placeholder `POST /api/v1/events` that is not wired to run execution yet

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
- HTTP endpoints: `/health`, `/metrics`, `/api/v1/events`

Local run:
- copy `.env.example` to `.env`
- start infra with `make infra-up`
- start the service with `go run ./apps/orchestrator`

Testing:
- unit and integration tests: `go test ./apps/orchestrator/...`
- full repo checks: `go test ./...`, `go build ./...`, `go vet ./...`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/run-lifecycle-spec.md`
- `docs/architecture/model-transport-contract.md`
- `docs/architecture/memory-model.md`

Current limitations:
- no model transport provider is wired yet
- `POST /api/v1/events` is still a placeholder boundary, not a working ingestion flow
- Dockerfile exists, but the full service stack is not wired into Compose yet
