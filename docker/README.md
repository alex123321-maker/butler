# Docker

Service Dockerfiles live here.

Current state:
- multistage Dockerfiles exist for the current Go service binaries;
- the Compose stack is still infra-first and does not yet wire every service into `deploy/docker-compose.yml`;
- `tool-http` and `tool-browser` Dockerfiles now package runnable runtime services, while other later tool services still remain stubs.

When added, each service Dockerfile should use a multistage build, keep dependency footprint small, and match the Docker Compose deployment model described in the architecture docs.
