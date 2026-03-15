# Apps

Go service binaries live here.

- `orchestrator/` hosts the Butler core API process, Session Service implementation, run lifecycle logic, model execution flow, and synchronous event ingestion APIs.
- `migrator/` applies and rolls back SQL migrations against PostgreSQL.
- `tool-broker/` now provides registry-backed validation and runtime routing for tool execution.
- `tool-browser/`, `tool-http/`, and `tool-doctor/` remain runtime binaries that will be filled out in later Sprint 4 and Sprint 5 tasks.

Each service directory must keep its own `README.md` with purpose, configuration, local run instructions, test instructions, and related architecture references.
