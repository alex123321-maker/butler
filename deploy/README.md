# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the baseline infrastructure services required for local Butler development.
- `docker-compose.dev.yml` adds local ports and persistent dev volumes.
- Copy `../.env.example` to `../.env` before running `make infra-up`.

Current Sprint 0 infra baseline includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues
