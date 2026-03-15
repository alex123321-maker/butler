# Butler

Self-hosted personal long-lived agent platform.

## Status

Sprint 0 through Sprint 7 are implemented in the repository backlog and reflected in the current codebase. The stack now includes the monorepo foundation, shared infra packages, contracts, migrations, Session Service, Redis leases, run lifecycle persistence, transcript storage, normalized ingress helpers, the OpenAI transport with Realtime WebSocket preference and SSE fallback, synchronous orchestrator ingestion APIs, Telegram delivery and approval UX, the full tool-enabled Docker Compose stack, Tool Broker validation and routing, credential metadata and deferred secret resolution, working/profile/episodic memory stores, the async memory extraction pipeline, session summaries, the doctor runtime, and the Nuxt web UI shell with sessions and doctor views.

## Repository layout

- `apps/` - Go service binaries such as the orchestrator and tool runtimes.
- `internal/` - shared Go packages used across services.
- `web/` - Nuxt.js frontend code.
- `proto/` - gRPC contract definitions and generated bindings.
- `migrations/` - SQL schema migrations.
- `docker/` - per-service Dockerfiles.
- `deploy/` - Docker Compose files and environment templates.
- `docs/` - architecture, planning, and engineering documentation.

## Local workflow

- Copy `.env.example` to `.env` before starting local infrastructure.
- `make build` - build all Go packages.
- `make test` - run the Go test suite.
- `make lint` - run baseline static checks with `go vet`.
- `make proto` - regenerate the current gRPC bindings from `proto/`.
- `make infra-up` - start only PostgreSQL and Redis for local `go run` workflows.
- `make infra-down` - stop the local infrastructure services.
- `make up` - build and start the Compose-based MVP stack.
- `make down` - stop the full Compose-based MVP stack.
- `make logs` - stream the unified Compose logs.

## Services

- `apps/orchestrator/` - Butler core API and orchestration service.
- `apps/tool-broker/` - validation and routing for tool calls.
- `apps/tool-browser/` - browser automation runtime.
- `apps/tool-http/` - HTTP runtime.
- `apps/tool-doctor/` - self-diagnostics runtime.

## Documentation

- [PRD + Architecture Specification](docs/architecture/butler-prd-architecture.md)
- [Memory Model](docs/architecture/memory-model.md)
- [Tooling and Execution Specification](docs/architecture/tool-runtime-adr.md)
- [Credential Management](docs/architecture/credential-management.md)
- [Run Lifecycle Specification](docs/architecture/run-lifecycle-spec.md)
- [Model Transport Contract](docs/architecture/model-transport-contract.md)
- [Implementation Roadmap](docs/planning/butler-implementation-roadmap.md)
- [Sprint Backlog](docs/planning/butler-backlog.yaml)

All architecture documents are written in Russian with English technical terms.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
