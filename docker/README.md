# Docker

Service Dockerfiles live here.

Current state:
- multistage Dockerfiles exist for all current backend services plus the Nuxt web UI
- `deploy/docker-compose.yml` wires the full local stack: `postgres`, `redis`, `migrator`, `orchestrator`, `tool-broker`, `tool-http`, `tool-browser`, `tool-doctor`, and `web`
- Go services run as a non-root `butler` user in the final image
- `tool-browser` uses the Playwright base image for browser automation support

Guidelines:
- keep dependency footprint small
- match the Docker Compose deployment model described in the architecture docs
- preserve the split between backend and internal tool-runtime networks
