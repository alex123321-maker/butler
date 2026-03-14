# Apps

Go service binaries live here.

- `orchestrator/` hosts the Butler core API process, Session Service implementation, run lifecycle logic, and current health/metrics bootstrap.
- `migrator/` applies and rolls back SQL migrations against PostgreSQL.
- `tool-broker/`, `tool-browser/`, `tool-http/`, and `tool-doctor/` are planned service binaries with Sprint 0 skeleton stubs only.

Each service directory must keep its own `README.md` with purpose, configuration, local run instructions, test instructions, and related architecture references.
