# Tool Broker

Tool Broker service for Butler tool validation, policy enforcement, credential mediation, and runtime routing.

Current state:
- gRPC server exposes `ListTools`, `GetToolContract`, `ValidateToolCall`, and routed `ExecuteToolCall`
- in-memory Tool Registry is loaded from `configs/tools.json`
- schema validation is performed against tool `input_schema_json`
- execution routing dials the per-tool `runtime_target` over the internal runtime gRPC contract

Expected dependencies:
- generated `toolbroker` gRPC bindings
- generated `runtime` gRPC bindings
- typed config from `internal/config`
- future credential resolution and richer runtime policy layers

Configuration:
- current typed config loader supports `BUTLER_TOOL_REGISTRY_PATH` and `BUTLER_TOOL_DEFAULT_TARGET`
- placeholder registry file lives at `configs/tools.json`
- shared service settings come from `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`, and `BUTLER_LOG_LEVEL`

Local run:
- `go run ./apps/tool-broker`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and gRPC on `BUTLER_GRPC_ADDR`
- Compose stack wires the broker to `tool-http` and `tool-browser` over the internal runtime network

Testing:
- run `go test ./apps/tool-broker/...`

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
