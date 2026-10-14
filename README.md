# Butler

Self-hosted personal long-lived agent platform.

## Status

Sprint 0 through Sprint 7 are implemented in the repository backlog and reflected in the current codebase. The stack now includes the monorepo foundation, shared infra packages, contracts, migrations, Session Service, Redis leases, run lifecycle persistence, transcript storage, normalized ingress helpers, the OpenAI transport with Realtime WebSocket preference and SSE fallback, additional OpenAI Codex and GitHub Copilot provider integrations with orchestrator-managed auth flows, synchronous orchestrator ingestion APIs, Telegram delivery and approval UX, the full tool-enabled Docker Compose stack, Tool Broker validation and routing, credential metadata and deferred secret resolution, working/profile/episodic memory stores, the async memory extraction pipeline, session summaries, the doctor runtime, and the Nuxt web UI shell with sessions and doctor views.

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
- `make build` or `make build-go` - build Butler Go packages only (`apps/`, `internal/`, `scripts/`) without walking `web/`.
- `make test` or `make test-go` - run the Go test suite without scanning frontend dependencies.
- `make lint` or `make lint-go` - run baseline static checks with `go vet` on Butler Go packages.
- `make build-web` - build the Nuxt frontend from `web/`.
- `make lint-web` - run frontend ESLint and Stylelint checks.
- `make test-web` - run Playwright end-to-end tests for the web UI.
- `make test-orchestrator` - run orchestrator-only Go tests.
- `make test-memory` - run memory subsystem Go tests.
- `make test-memory-eval` - run the small mandatory offline memory-evaluation contract suite; [laba reproduction and model-trial limits](cmd/memory-eval/README.md).
- `make test-transport` - run transport/provider Go tests.
- `make proto` - regenerate the current gRPC bindings from `proto/`.
- `make infra-up` - start only PostgreSQL and Redis for local `go run` workflows.
- `make infra-down` - stop the local infrastructure services.
- `make up` - build and start the Compose-based MVP stack.
- `make down` - stop the full Compose-based MVP stack.
- `make logs` - stream the unified Compose logs.

## Services

- `apps/orchestrator/` - Butler core API and orchestration service.
- `apps/restart-helper/` - allowlisted Docker Compose restart helper for Settings-driven service restarts.
- `apps/tool-broker/` - validation and routing for tool calls.
- `apps/tool-browser/` - browser automation runtime.
- `apps/tool-http/` - HTTP runtime.
- `apps/tool-doctor/` - self-diagnostics runtime.
- `apps/tool-extension/` - optional reviewed HTTP-JSON executor with separate
  runtime authentication, public resource bounds and no package-code execution.
- `apps/memory-parser/` - optional fixed PDF/text/OCR parser in a separate no-egress sandbox.

## Documentation

- [PRD + Architecture Specification](docs/architecture/butler-prd-architecture.md)
- [Memory Model](docs/architecture/memory-model.md)
- [Versioned memory sources, privacy and structured ingestion](docs/architecture/versioned-memory-sources.md)
- [Tooling and Execution Specification](docs/architecture/tool-runtime-adr.md)
- [Credential Management](docs/architecture/credential-management.md)
- [Run Lifecycle Specification](docs/architecture/run-lifecycle-spec.md)
- [Model Transport Contract](docs/architecture/model-transport-contract.md)
- [AI Repo Map](docs/ai/repo-map.md)
- [Implementation Roadmap](docs/planning/butler-implementation-roadmap.md)
- [Sprint Backlog](docs/planning/butler-backlog.yaml)

All architecture documents are written in Russian with English technical terms.

Reviewed extension installation is available through the authenticated
`/extensions` owner page. Installation, review, enablement, readiness and task
access stay separate. Start with the [optional installation guide](deploy/extensions/README.md)
and [lifecycle contract](docs/architecture/reviewed-extension-lifecycle.md).
The reference project connector prepares one synthetic README artifact with
recipient proof; it does not modify a real repository or run a build.

The opt-in [pure generated converter](docs/architecture/generated-converters.md)
can create/check/trial and activate an exact bounded delimited-to-JSON IR under
current owner permission. Its production runtime has no network and no core/account
credentials. `/converters` shows checks, finite rights and current output evidence.
Actual delegated developer acceptance remains a separate integration gate.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
