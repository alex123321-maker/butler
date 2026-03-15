# Apps

Go service binaries live here.

- `orchestrator/` hosts the Butler core API process, Session Service implementation, run lifecycle logic, model execution flow, and synchronous event ingestion APIs.
- `migrator/` applies and rolls back SQL migrations against PostgreSQL.
- `tool-broker/` provides registry-backed validation, credential mediation, policy checks, and runtime routing for tool execution.
- `tool-http/` provides the HTTP-class runtime tools: `http.request`, `http.download`, and `http.parse_html`.
- `tool-browser/` provides the browser-class runtime tools including navigation, DOM interaction, extraction, and storage helpers.
- `tool-doctor/` provides the doctor runtime tools for configuration, database, container, and provider diagnostics.

Each service directory must keep its own `README.md` with purpose, configuration, local run instructions, test instructions, and related architecture references.
