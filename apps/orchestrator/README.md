# Orchestrator

Butler core API and orchestration service.

Current Sprint 0 baseline:
- loads typed config from env
- initializes shared structured logging
- connects to PostgreSQL and Redis on startup
- exposes `GET /health`
- exposes `GET /metrics`
- exposes placeholder `POST /api/v1/events`

Local run:
- ensure infrastructure is up with `make infra-up`
- copy `.env.example` to `.env`
- start the service with `go run ./apps/orchestrator`
