# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the internal Butler stack used as the base for all local environments.
- `docker-compose.dev.yml` adds host-exposed ports and persistent dev volumes required for browser and direct local access.
- Copy `../.env.example` to `../.env` before running `make up` or `make infra-up`.
- Compose no longer injects default values for Butler config fields; if an env var is absent, the service falls back to Butler's own config resolution (env -> DB override -> code default).

Current Compose stack includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues
- `migrator` for schema setup
- `orchestrator` for ingress, run execution, memory context loading, and transport
- `restart-helper` for narrow helper-managed Docker Compose restarts of allowlisted Butler services
- `tool-broker` for tool validation, credential policy checks, secret resolution, and runtime routing
- `tool-http`, `tool-browser`, and `tool-doctor` as isolated runtime services
- `web` for the Nuxt operator UI

Current behavior notes:
- runtime isolation is network-based inside Compose through the internal `tool-runtime` network
- `restart-helper` is intentionally attached only to the internal backend network and uses an explicit allowlist plus self-exclusion before touching the Docker socket
- `tool-doctor` does not get the Docker socket and limits `doctor.check_container` to probing configured service health URLs
- for local browser usage, start Compose with both files or use `make up`; the base file alone does not publish the orchestrator REST API to the host
- the stack wires fixed internal Compose URLs for PostgreSQL, Redis, Tool Broker, and web-to-orchestrator connectivity; optional Butler settings are passed through only when they exist in the host environment
- optional variables such as `BUTLER_OPENAI_API_KEY` and Telegram credentials are passed through only when they exist in the host environment, so missing values do not appear as empty env overrides inside containers
- browser, HTTP, and doctor runtimes are runnable V1 services rather than skeleton placeholders
