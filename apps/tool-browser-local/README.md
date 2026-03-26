# Tool Browser Local

Local runtime for Butler `single_tab.*` tools.

Current state:
- exposes the internal `ToolRuntimeService` gRPC contract
- talks to orchestrator over HTTP for single-tab session lookup, state updates, and release
- dispatches single-tab browser actions through the local `browser-bridge` control API
- supports `single_tab.bind` by requesting extension-driven tab discovery through orchestrator and creating durable `browser_tab_selection` approvals
- supports working `single_tab.status` and `single_tab.release`
- supports `single_tab.navigate`, `single_tab.reload`, `single_tab.go_back`, `single_tab.go_forward`, `single_tab.click`, `single_tab.fill`, `single_tab.type`, `single_tab.press_keys`, `single_tab.scroll`, `single_tab.wait_for`, `single_tab.extract_text`, and `single_tab.capture_visible` when the Chromium extension is connected
- materializes `single_tab.capture_visible` results into durable Butler artifacts and returns an artifact-backed `image_ref`
- resolves follow-up `single_tab.*` actions against the active session for the current Butler `session_key` when the model omits `session_id` or accidentally passes the Butler session key instead of the durable `single_tab_session_id`
- automatically requests a recovery `single_tab.bind` flow and retries the original action once when a follow-up `single_tab.*` action fails because the extension heartbeat went stale, the bound tab closed, or the active tab session must be re-established
- can run on host or in Compose when using `orchestrator_relay` dispatch (extension-connected remote flow)

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
