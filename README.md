# Butler

Self-hosted personal long-lived agent platform.

## Status

Sprint 0 foundation is in progress. The repository now includes the initial Go module, service stubs, shared top-level directories, and baseline automation targets.

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
- `make proto` - placeholder target for proto generation wiring added in later Sprint 0 tasks.
- `make infra-up` - start local PostgreSQL and Redis once Compose assets exist.
- `make infra-down` - stop local infrastructure.

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
- [Sprint Backlog](docs/planning/butler-backlog-sprint-0-2.yaml)

All architecture documents are written in Russian with English technical terms.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
