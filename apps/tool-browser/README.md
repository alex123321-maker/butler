# Tool Browser

Browser runtime service for Butler browser automation tools.

Current state:
- runtime exposes the internal `ToolRuntimeService` gRPC contract
- supports `browser.navigate`, `browser.snapshot`, `browser.click`, `browser.fill`, `browser.type`, `browser.wait_for`, `browser.extract_text`, `browser.set_cookie`, and `browser.restore_storage_state`
- URL and selector validation are enforced before execution, and extracted text is truncated to safe response sizes
- resolved credential refs can be injected for fill/type flows without exposing raw secrets to model-visible args
- Playwright helper resolution is deployment-safe through `BUTLER_TOOL_BROWSER_SCRIPT_PATH` instead of `runtime.Caller` path discovery
- HTTP health endpoint and unit tests are in place

Local run:
- `go run ./apps/tool-browser`
- requires `npm install` in `apps/tool-browser/` for the Playwright helper
- service exposes HTTP health on `BUTLER_HTTP_ADDR` and runtime gRPC on `BUTLER_GRPC_ADDR`
- optional overrides: `BUTLER_TOOL_BROWSER_NODE_BINARY`, `BUTLER_TOOL_BROWSER_SCRIPT_PATH`

Intended responsibilities:
- execute browser-class tools only
- stay behind Tool Broker routing and policy checks
- never receive raw secrets directly from model-visible context

Related docs:
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/planning/butler-implementation-roadmap.md`
