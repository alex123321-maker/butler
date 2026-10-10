# Tool Browser

Browser runtime service for Butler browser automation tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- `browser.read` independently reads and destroys a temporary context; `browser.open` creates an explicit run-owned session for sequences, and `browser.close` destroys it
- navigate/click/fill/type/wait/auth actions require the returned session ID, latest observation version and expected current URL; snapshot/extract never navigate
- each context has a fixed origin, 15-minute TTL and exclusive operation lock; 32 live contexts maximum, with no cross-run transfer or implicit crash recovery
- one long-lived Playwright helper keeps contexts between calls through private stdin/stdout messages; resolved secrets are never process arguments or diagnostics
- URL and selector validation are enforced before execution, and extracted text is truncated to safe response sizes
- password inputs require broker-resolved refs; cookie/storage inputs accept refs only, retain separate approval and cannot enlarge the session origin
- auth persists within its live context; cookies/storage/password values are never exported and reflected secrets are masked in observations
- credential actions use the verified exact-version Broker approval handoff; credential resolution rechecks the actual current URL and fixed origin before injection
- Playwright helper resolution is deployment-safe through `BUTLER_TOOL_BROWSER_SCRIPT_PATH` instead of `runtime.Caller` path discovery
- Docker Compose deployment must attach the runtime to an egress-capable network in addition to the internal tool network so outbound site navigation can resolve public DNS
- HTTP health endpoint and unit tests are in place
- real Chromium e2e checks form continuation, auth persistence/masking, stale observations, expiry and process loss via `scripts/dev/lab-browser.sh`; use only the isolated lab workflow
- all requests/resources are restricted to the fixed origin, so cross-origin SSO/CDNs can be unavailable; `single_tab.*` remains separately approved and unchanged

Local run:
- `go run ./apps/tool-browser`
- requires `npm ci --ignore-scripts` with the committed package-lock in `apps/tool-browser/`
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`
- optional overrides: `BUTLER_TOOL_BROWSER_NODE_BINARY`, `BUTLER_TOOL_BROWSER_SCRIPT_PATH`

Intended responsibilities:
- execute browser-class tools only
- stay behind Tool Broker routing and policy checks
- never receive raw secrets directly from model-visible context

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/browser-session-contract.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`

The reviewed helper is embedded in Go and checked against its workspace file; arbitrary helper updates fail before Node startup. Private expiring proxies pin public DNS and origin, share a 4 MiB outbound counter, deny WebSockets/service workers and allow writes only during authorized interactions. The image is read-only with temporary mounts and receives no model/PG credentials. Real Chromium tests include malicious POST/private iframe/foreign fetch/WebSocket attempts with zero effects, alongside permitted forms and auth. See [security boundaries](../../docs/architecture/agent-security-boundaries.md).
