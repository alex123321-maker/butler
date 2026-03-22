# Restart Helper

Narrow control-plane helper that can restart Butler Docker Compose services without giving the orchestrator unrestricted Docker control.

Current responsibilities:
- exposes a tiny HTTP API for scheduling restart of allowlisted Compose services
- talks to the Docker Engine API through the mounted Docker socket
- validates requested services against an explicit allowlist
- refuses to restart its own `restart-helper` container even if it appears in configuration
- schedules restart asynchronously so the caller can answer the web request before `web` or `orchestrator` go down
- exposes `/health` with a Docker ping check

Boundaries:
- no access to Butler memory, transcripts, runs, approvals, or credentials
- no arbitrary Docker command execution
- no container creation, deletion, image pulls, or Compose graph mutation
- no self-restart
- no diagnostics role; service health inspection remains in `tool-doctor`
- intended caller is the orchestrator settings API over the internal Compose network

Configuration:
- `BUTLER_HTTP_ADDR` HTTP listen address
- `BUTLER_DOCKER_HOST` Docker Engine endpoint, default `unix:///var/run/docker.sock`
- `BUTLER_RESTART_PROJECT` Compose project name, default `butler`
- `BUTLER_RESTART_ALLOWED_SERVICES` comma-separated allowlist of restartable services
- `BUTLER_RESTART_SELF_SERVICE` service name to exclude from restart, default `restart-helper`
- `BUTLER_RESTART_DELAY_SECONDS` async scheduling delay before restart begins
- `BUTLER_RESTART_TIMEOUT_SECONDS` per-service Docker restart timeout

HTTP API:
- `POST /v1/restarts`
  - body: `{"services":["orchestrator","web"]}`
  - returns `202 Accepted` with scheduled services, delay, and a suggested manual fallback command
- `GET /health`

Local run:
- this service is intended to run inside Docker Compose with `/var/run/docker.sock` mounted
- standalone example:
  - `go run ./apps/restart-helper`

Testing:
- `go test ./apps/restart-helper/...`

Related docs:
- `docs/architecture/butler-prd-architecture.md`
- `docs/ai/engineering-rules.md`
- `deploy/README.md`
