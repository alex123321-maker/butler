# Tool HTTP

Planned HTTP runtime service for Butler web and API tools.

Current state:
- Sprint 0 skeleton only; `main.go` is an empty stub
- no runtime contract implementation, allowlist policy layer, or tests yet

Local run:
- `go run ./apps/tool-http`
- current stub exits immediately because runtime wiring does not exist yet

Intended responsibilities:
- execute HTTP-class tools only
- stay behind Tool Broker routing and policy checks
- support credential-aware execution through broker-mediated resolution only

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
