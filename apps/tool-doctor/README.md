# Tool Doctor

Planned doctor runtime service for Butler diagnostics and self-inspection.

Current state:
- Sprint 0 skeleton only; `main.go` is an empty stub
- config introspection, health, logging, and metrics foundations already exist in shared packages
- no doctor runtime RPC implementation, report persistence, or tests yet

Local run:
- `go run ./apps/tool-doctor`
- current stub exits immediately because runtime wiring does not exist yet

Intended responsibilities:
- inspect effective configuration without exposing secrets
- check infrastructure, provider, and container health
- return actionable operator-safe diagnostic reports

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/architecture/tool-runtime-adr.md`
- `docs/planning/butler-implementation-roadmap.md`
