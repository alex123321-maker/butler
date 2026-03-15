# Tool Browser

Planned browser runtime service for Butler browser automation tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- `browser.navigate` and `browser.snapshot` run through a Playwright helper
- HTTP health endpoint and unit tests are in place

Local run:
- `go run ./apps/tool-browser`
- requires `npm install` in `apps/tool-browser/` for the Playwright helper
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`

Intended responsibilities:
- execute browser-class tools only
- stay behind Tool Broker routing and policy checks
- never receive raw secrets directly from model-visible context

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
