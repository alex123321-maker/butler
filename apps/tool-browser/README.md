# Tool Browser

Planned browser runtime service for Butler browser automation tools.

Current state:
- Sprint 0 skeleton only; `main.go` is an empty stub
- no runtime contract implementation, browser backend, health endpoint, or tests yet

Local run:
- `go run ./apps/tool-browser`
- current stub exits immediately because runtime wiring does not exist yet

Intended responsibilities:
- execute browser-class tools only
- stay behind Tool Broker routing and policy checks
- never receive raw secrets directly from model-visible context

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
