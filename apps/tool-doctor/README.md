# Tool Doctor

Doctor runtime service for Butler diagnostics and self-inspection.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- `doctor.check_system` returns a secret-safe report with config validation plus database checks
- `doctor.check_database`, `doctor.check_container`, and `doctor.check_provider` are available through the same runtime contract
- config introspection, health, and logging foundations are wired into the runtime report
- unit tests cover the checker and runtime server

Local run:
- `go run ./apps/tool-doctor`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`
- optional dependency checks use `BUTLER_POSTGRES_URL`, `BUTLER_REDIS_URL`, `BUTLER_OPENAI_API_KEY`, and `BUTLER_DOCTOR_CONTAINER_TARGETS`

Intended responsibilities:
- inspect effective configuration without exposing secrets
- check infrastructure, provider, and container health
- return actionable operator-safe diagnostic reports

Compose:
- the Docker stack now includes `tool-doctor` and registers `doctor.check_system`, `doctor.check_database`, `doctor.check_container`, and `doctor.check_provider` in `configs/tools.json`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/tool-runtime-adr.md`
- `docs/planning/butler-implementation-roadmap.md`
