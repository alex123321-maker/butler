# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the baseline infrastructure services required for local Butler development.
- `docker-compose.dev.yml` adds local ports and persistent dev volumes.
- Copy `../.env.example` to `../.env` before running `make infra-up`.

Current infra baseline includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues

Current limitations:
- the Compose stack is infra-only and does not yet include `orchestrator` or tool services;
- service Dockerfiles now exist under `docker/`, but service wiring in Compose is still follow-up work;
- this directory therefore reflects Sprint 0 deployment readiness, not a full MVP stack.
