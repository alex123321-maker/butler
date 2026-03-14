# Tool Broker

Planned Tool Broker service for Butler tool validation, policy enforcement, credential mediation, and runtime routing.

Current state:
- Sprint 0 skeleton only; `main.go` is an empty stub
- proto contracts exist under `proto/toolbroker/v1`
- no gRPC server, registry loader, validation logic, or routing yet

Expected dependencies:
- generated `toolbroker` gRPC bindings
- typed config from `internal/config`
- future credential and runtime contracts

Configuration:
- current typed config loader supports `BUTLER_TOOL_REGISTRY_PATH` and `BUTLER_TOOL_DEFAULT_TARGET`
- placeholder registry file lives at `configs/tools.json`

Local run:
- `go run ./apps/tool-broker`
- current stub exits immediately because no server wiring exists yet

Testing:
- no service-specific tests yet; current repo-wide validation is limited to compile/test baselines

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
