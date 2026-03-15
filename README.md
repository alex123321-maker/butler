# Butler

Self-hosted personal long-lived agent platform.

## Status

Sprint 0 and Sprint 1 are complete. Sprint 2 is functionally delivered for the first vertical slice, and Sprint 3 is also complete with the first user-facing MVP path. Sprint 4 is now in progress, starting with the internal gRPC runtime contract used between the Tool Broker and isolated runtime containers. The OpenAI transport prefers Realtime WebSocket with HTTP streaming (SSE) fallback while preserving the same Butler transport contract. The repository now includes the monorepo foundation, shared infra packages, contracts, migrations, Session Service, Redis leases, run lifecycle persistence, transcript storage, normalized ingress helpers, normalized transport types, the OpenAI transport path, synchronous orchestrator event ingestion APIs, the in-process Telegram adapter, the MVP Docker Compose stack, the Tool Broker skeleton, the Working Memory baseline, the runtime gRPC contract, and documented smoke/end-to-end verification flows.

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
- [Implementation Roadmap](docs/planning/butler-implementation-roadmap.md)
- [Sprint Backlog](docs/planning/butler-backlog.yaml)

All architecture documents are written in Russian with English technical terms.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
