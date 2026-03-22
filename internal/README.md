# Internal

Shared Go packages live here.

Package map:
- `config/` - typed config loading, layered settings, secret masking, hot config.
- `credentials/` - credential metadata, policy checks, audit, deferred tool credential resolution.
- `domain/` - domain enums, errors, and proto/domain conversion helpers.
- `gen/` - generated gRPC/protobuf bindings; edit `proto/` instead of hand-editing generated files.
- `health/` - shared health-check primitives used by services and diagnostics.
- `ingress/` - normalized inbound event types and helpers for channel input.
- `logger/` - structured logging helpers.
- `memory/` - transcript, working, profile, episodic, chunks, provenance, retrieval, and pipeline logic.
- `metrics/` - shared Prometheus metric registration.
- `modelprovider/` - provider selection enums and shared provider metadata.
- `providerauth/` - provider auth flows and persisted auth state.
- `providerfactory/` - transport/provider wiring for orchestrator bootstrap.
- `storage/` - PostgreSQL and Redis client wrappers.
- `transport/` - provider-normalized model transport contracts and implementations.

Routing notes:
- Change `proto/` and regenerate bindings when an internal gRPC contract changes.
- Change `storage/` only for shared database or Redis access concerns; business logic should stay in service packages.
- Change `memory/` only after reading the corresponding memory architecture docs for non-trivial tasks.
