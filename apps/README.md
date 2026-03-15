# Apps

Go service binaries live here.

- `orchestrator/` hosts the Butler core API process, Session Service implementation, run lifecycle logic, model execution flow, and synchronous event ingestion APIs.
- `migrator/` applies and rolls back SQL migrations against PostgreSQL.
- `tool-broker/` now provides registry-backed validation and runtime routing for tool execution.
- `tool-http/` now provides the first runtime service for `http.request` execution.
- `tool-browser/` now provides the first browser runtime service for `browser.navigate` and `browser.snapshot`.
- `tool-doctor/` remains a later runtime binary for Sprint 5.

Each service directory must keep its own `README.md` with purpose, configuration, local run instructions, test instructions, and related architecture references.
