# Proto

Internal gRPC contracts live here.

- `common/v1` defines shared enums and core typed values used across services.
- `approval/v1` defines typed approval payloads such as browser tab selection.
- `browser/v1` defines session-bound single-tab browser control contracts.
- `orchestrator/v1` defines the normalized ingress API exposed by the orchestrator.
- `run/v1` defines normalized ingress events.
- `runtime/v1` defines the internal tool runtime execution contract.
- `session/v1` defines the Session Service and durable run lifecycle API.
- `toolbroker/v1` defines Tool Broker registry, validation, and execution contracts.
- `transport/v1` defines the logical Model Transport contract used by orchestrator-facing providers.
- `webfetch/v1` defines provider-abstracted web retrieval and extraction contracts.

Regenerate Go bindings with `make proto`. Generated Go code is written under `internal/gen/`.
