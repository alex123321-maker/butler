# Deploy

Docker Compose files and environment templates live here.

- `docker-compose.yml` defines the current local Butler stack, including orchestrator, tool-broker, and the first tool runtimes.
- `docker-compose.dev.yml` adds local ports and persistent dev volumes.
- Copy `../.env.example` to `../.env` before running `make infra-up`.

Current Compose baseline includes:
- `postgres` using a pgvector-enabled image
- `redis` for transient state and queues
- `orchestrator` for ingress, run execution, and transport
- `tool-broker` for tool validation and routing
- `tool-http` and `tool-browser` as isolated runtime services

Current limitations:
- the Compose stack still does not include every later Butler service such as `tool-doctor` or the Web UI;
- browser and HTTP runtimes are skeletons rather than full V1 capability sets;
- runtime isolation is currently network-based inside Compose and will keep evolving with later sprint work.
