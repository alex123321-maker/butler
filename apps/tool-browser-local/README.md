# Tool Browser Local

Local runtime for Butler `single_tab.*` tools.

Current state:
- exposes the internal `ToolRuntimeService` gRPC contract
- talks to orchestrator over HTTP for single-tab session lookup, state updates, and release
- dispatches single-tab browser actions through the local `browser-bridge` control API
- supports working `single_tab.status` and `single_tab.release`
- supports `single_tab.navigate`, `single_tab.reload`, `single_tab.go_back`, `single_tab.go_forward`, `single_tab.click`, `single_tab.fill`, `single_tab.type`, `single_tab.press_keys`, `single_tab.scroll`, `single_tab.wait_for`, `single_tab.extract_text`, and `single_tab.capture_visible` when the Chromium extension is connected
- materializes `single_tab.capture_visible` results into durable Butler artifacts and returns an artifact-backed `image_ref`
- is intended to run on the host, near the user's real browser, not as a Compose-only runtime

Environment:
- `BUTLER_TOOL_BROWSER_LOCAL_ORCHESTRATOR_URL`
- `BUTLER_TOOL_BROWSER_LOCAL_BROWSER_BRIDGE_URL`
- `BUTLER_TOOL_BROWSER_LOCAL_ROLLOUT_MODE` (`native_only`, `dual`, `remote_preferred`)
- `BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE` (`browser_bridge` or `orchestrator_relay`)
- `BUTLER_TOOL_BROWSER_LOCAL_REQUEST_TIMEOUT_SECONDS`
- standard shared runtime vars: `BUTLER_SERVICE_NAME`, `BUTLER_LOG_LEVEL`, `BUTLER_HTTP_ADDR`, `BUTLER_GRPC_ADDR`

Local run:
- `go run ./apps/tool-browser-local`

Dispatch modes:
- `native_only` rollout mode forces local `browser_bridge` dispatch, regardless of `BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE`
- `dual` rollout mode uses `BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE` as explicit selector
- `remote_preferred` rollout mode forces `orchestrator_relay` dispatch to `/api/v2/single-tab/actions/dispatch`
