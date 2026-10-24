# Tool Doctor

Doctor runtime service for Butler diagnostics and self-inspection.

The owner Doctor API composes communication metadata/readiness from the owning
Orchestrator, separately from this runtime's system checks. Channel failure or
an unavailable observation cannot be hidden by healthy database checks. This
runtime gains no communication database, content, channel client or execution
rights. See [communication diagnostics](../../docs/operations/communication-diagnostics.md).

Operator launches now use Orchestrator's authenticated durable input and exact
Broker authorization path; the runtime receives a real run context. Its
`doctor.check_system` registry contract is a read-only observation with
`read_result` evidence. A completed tool response is distinct from the diagnostic
status inside its report. Provider configuration checks still do not prove live
inference quality. See the [runbook](../../docs/operations/memory-observability.md).

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- `doctor.check_system` returns a secret-safe report with config validation plus database checks
- `doctor.check_database`, `doctor.check_container`, and `doctor.check_provider` are available through the same runtime contract
- config introspection, health, and logging foundations are wired into the runtime report
- unit tests cover the checker and runtime server

Local run:
- `go run ./apps/tool-doctor`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`
- dependency checks use fixed `BUTLER_DOCTOR_CORE_DIAGNOSTICS_URL` and
  `BUTLER_DOCTOR_CONTAINER_TARGETS`; Doctor receives no PostgreSQL, Redis or model
  credentials. Core performs its own dependency checks and returns bounded,
  secret-free status enums. Provider status means credentials are configured,
  not that real inference has been tested. Redirects are denied.

Intended responsibilities:
- inspect effective configuration without exposing secrets
- check infrastructure, provider, and container health via configured health endpoints
- return actionable operator-safe diagnostic reports

Explicit non-responsibilities:
- no Docker socket access
- no container restart, stop, or lifecycle control
- no overlap with `restart-helper`, which owns allowlisted Docker restart operations

Compose:
- the Docker stack now includes `tool-doctor` and registers `doctor.check_system`, `doctor.check_database`, `doctor.check_container`, and `doctor.check_provider` in `configs/tools.json`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/tool-runtime-adr.md`
- `docs/planning/butler-implementation-roadmap.md`
