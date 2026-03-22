# Browser Bridge

Host-installed native messaging companion for single-tab browser control.

Current responsibilities:
- reads and writes Chromium native messaging frames over `stdin` / `stdout`
- keeps `stdout` reserved for protocol messages and logs to `stderr`
- exposes a local control API for `tool-browser-local` action dispatch
- proxies browser-tab bind requests into orchestrator via `/api/v2/single-tab/bind-requests`
- queries the active single-tab session via `/api/v2/single-tab/session`
- stays outside Docker Compose by design; this is a host companion, not a compose runtime

Current native messaging methods:
- `ping`
- `bind.request`
- `session.get_active`
- host-initiated `action.dispatch` over the persistent native port

Expected native host name for Chromium-based browsers:
- `com.butler.browser_bridge`

Environment:
- `BUTLER_BROWSER_BRIDGE_ORCHESTRATOR_URL`
- `BUTLER_BROWSER_BRIDGE_CONTROL_ADDR`
- `BUTLER_BROWSER_BRIDGE_REQUEST_TIMEOUT_SECONDS`
- standard shared logging keys: `BUTLER_SERVICE_NAME`, `BUTLER_LOG_LEVEL`, `BUTLER_ENVIRONMENT`

Local run:
- `go run ./apps/browser-bridge`

Local control API:
- `POST /api/v1/actions/dispatch`
- `GET /health`

Notes:
- this service does not make orchestration decisions
- tab approval still resolves through orchestrator durable approvals and channel delivery
- future extension/native-host work can reuse this process without changing orchestrator contracts

Native host manifest:
- example template: `apps/browser-bridge/examples/chromium-native-host.manifest.json`
- replace the binary `path` with the absolute path to your local `browser-bridge` executable
- replace `REPLACE_WITH_EXTENSION_ID` with the actual unpacked extension ID shown by Chrome/Chromium/Edge
- Windows helper: `pwsh -NoProfile -File scripts/dev/install-browser-bridge-host.ps1 -Browser Chrome -ExtensionId <extension_id>`
