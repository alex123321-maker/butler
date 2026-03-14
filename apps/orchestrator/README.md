# Orchestrator

Butler core API and orchestration service.

Current baseline:
- loads typed config from env
- initializes shared structured logging
- connects to PostgreSQL and Redis on startup
- serves the Session Service gRPC contract on `BUTLER_GRPC_ADDR`
- implements `CreateSession`, `GetSession`, and `ResolveSessionKey` inside the orchestrator process
- implements Redis-backed `AcquireLease`, `RenewLease`, and `ReleaseLease` session ownership operations
- implements durable run creation, lookup, and lifecycle transitions backed by PostgreSQL
- exposes `GET /health`
- exposes `GET /metrics`
- exposes placeholder `POST /api/v1/events`

Local run:
- ensure infrastructure is up with `make infra-up`
- copy `.env.example` to `.env`
- start the service with `go run ./apps/orchestrator`
