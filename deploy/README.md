# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the internal Butler stack used as the base for all local environments.
- `docker-compose.dev.yml` adds host-exposed ports and persistent dev volumes required for browser and direct local access.
- Copy `../.env.example` to `../.env` before running `make up` or `make infra-up`.

Current Compose stack includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues
- `migrator` for schema setup
- `orchestrator` for ingress, run execution, memory context loading, and transport
- `tool-broker` for tool validation, credential policy checks, secret resolution, and runtime routing
- `tool-http`, `tool-browser`, and `tool-doctor` as isolated runtime services
- `web` for the Nuxt operator UI

Current behavior notes:
- runtime isolation is network-based inside Compose through the internal `tool-runtime` network
- for local browser usage, start Compose with both files or use `make up`; the base file alone does not publish the orchestrator REST API to the host
- the default stack wires `BUTLER_POSTGRES_URL` into both `orchestrator` and `tool-broker`, so credential metadata and audit-aware policy checks work in the local deployment model
- browser, HTTP, and doctor runtimes are runnable V1 services rather than skeleton placeholders
