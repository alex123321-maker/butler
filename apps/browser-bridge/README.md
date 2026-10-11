# Browser Bridge

The original companion below is a legacy route. Additive
`./apps/browser-bridge/cmd/guarded` is disabled until operator composition; it
exposes authenticated resource-ticket gRPC with guarded status/click frames and
no legacy dispatch HTTP/bind API. `BUTLER_GUARDED_BROWSER_TOKEN_FILE` must be a
regular private Unix file (mode 0600), `BUTLER_GUARDED_BROWSER_AUTHORITY_URL` a fixed
private endpoint, and `BUTLER_GUARDED_BROWSER_GRPC_ADDR` defaults to 127.0.0.1:19090.
Only explicit loopback IP listeners are accepted. Remote Broker access needs an
operator-owned authenticated tunnel/gateway. Windows private-file ACL checks
remain pending; permissions are not weakened to permit an unproven install.
The token serves private Broker/host/authority hops only. Extension/model args
receive no token. Optional `BUTLER_GUARDED_BROWSER_METADATA_ADDR` exposes only
authenticated, bounded list/check on a separate loopback listener. Owning
`internal/singletabcontrol` atomically publishes approved opaque selections.
Actual Linux native-manifest/owner-library/Broker integration is tested;
production Web/Telegram selection after releasing a worker lease remains
pending. See the [guarded contract](../../docs/architecture/single-tab-resource-boundary.md)
and [composition design](../../docs/architecture/single-tab-production-composition.md).

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
