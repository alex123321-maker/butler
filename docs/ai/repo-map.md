# Butler AI Repo Map

Short routing guide for AI agents working in this repository.

## Start Here

- Read `AGENTS.MD` for repository-wide invariants and safety rules.
- Read `docs/ai/engineering-rules.md` before non-trivial changes.
- Use service `README.md` files for entrypoints, config, and verification commands.

## Subsystem Map

- `apps/orchestrator/`
  Butler core API, Session Service implementation, run execution flow, delivery, prompt assembly, approval handling.
- `apps/tool-broker/`
  Tool contract validation, runtime routing, credential-aware execution.
- `apps/tool-browser/`, `apps/tool-http/`, `apps/tool-doctor/`
  Concrete tool runtimes. Keep execution logic here, not orchestration.
- `internal/config/`
  Typed env loading, layered settings, hot config, secret masking.
- `internal/credentials/`
  Credential metadata, broker policy checks, audit, deferred secret resolution.
- `internal/memory/`
  Transcript store, working memory, episodic/profile memory, chunks, provenance, retrieval, async pipeline.
- `internal/transport/`
  Provider-normalized transport contracts and provider implementations.
- `internal/storage/`
  Shared PostgreSQL and Redis access helpers.
- `proto/`
  Source of truth for internal gRPC contracts.
- `migrations/`
  Durable schema changes. Keep SQL migrations and affected docs aligned.
- `web/`
  Nuxt operator UI. API entity modules live in `entities/`, shared state in `shared/model/stores/`, routes in `pages/`.

## If You Change X

- Run lifecycle, approvals, orchestration, sessions:
  Read `docs/architecture/run-lifecycle-spec.md` and inspect `apps/orchestrator/internal/orchestrator`, `apps/orchestrator/internal/run`, `apps/orchestrator/internal/session`.
- Memory retrieval, extraction, provenance:
  Read `docs/architecture/memory-model.md` and inspect `internal/memory/...`.
- Model/provider transport:
  Read `docs/architecture/model-transport-contract.md` and inspect `internal/transport/...`, `internal/providerauth/...`, `internal/providerfactory/...`.
- Tool contracts, broker policy, runtimes:
  Read `docs/architecture/tool-runtime-adr.md` and inspect `apps/tool-broker/...`, `apps/tool-*`, `configs/tools.json`.
- Credentials, secrets, approval policy:
  Read `docs/architecture/credential-management.md` and inspect `internal/credentials/...`.
- Settings or runtime config:
  Read `docs/architecture/config-layering.md` and inspect `internal/config/...`.
- Web UI task-centric screens:
  Inspect `web/pages/...`, `web/entities/...`, `web/shared/model/stores/...`, and the matching orchestrator API handlers in `apps/orchestrator/internal/api/...`.

## Verification Shortcuts

- Backend build:
  `make build-go`
- Backend tests:
  `make test-go`
- Backend lint:
  `make lint-go`
- Orchestrator-only tests:
  `make test-orchestrator`
- Memory-only tests:
  `make test-memory`
- Transport-only tests:
  `make test-transport`
- Frontend lint:
  `make lint-web`
- Frontend end-to-end tests:
  `make test-web`

## Noise To Ignore

Avoid spending context on:
- `web/node_modules/`
- `web/.nuxt/`
- `web/.output/`
- `.opencode/node_modules/`
- `internal/gen/` unless debugging generated output
- Playwright snapshot directories unless the task is explicitly visual
