# Chromium Butler Extension

Manifest V3 scaffold for the Butler single-tab browser bridge flow.

Current responsibilities:
- enumerates HTTP(S) tabs from the local Chromium-based browser
- sends `bind.request` messages to the host-installed `browser-bridge` native messaging companion
- checks the currently active `single_tab_session` for a Butler session key
- executes host-routed `single_tab.navigate`, `reload`, `go_back`, `go_forward`, `click`, `fill`, `type`, `press_keys`, `scroll`, `wait_for`, `extract_text`, and `capture_visible` requests inside the bound tab
- provides a minimal popup UI for manual development testing

Current limitations:
- native messaging host installation remains manual when `native` transport is selected
- approval selection still happens in Butler web UI or Telegram, not inside the extension

Notes:
- `capture_visible` returns a screenshot to Butler, and the host runtime persists it as a durable `browser_capture` artifact before returning the final `image_ref`.

Expected native messaging host:
- `com.butler.browser_bridge`

Target connection modes:
- `remote` (default): extension <-> Butler API (HTTPS + extension auth) with server-side relay for action dispatch
- `native` (optional fallback): extension <-> native messaging host (`browser-bridge`) <-> Butler API

Remote mode design constraints:
- extension auth must be machine-to-machine and must not depend on web UI cookies
- single-tab action dispatch must still be broker-routed and session-guarded
- relay failures must surface as normalized `HOST_UNAVAILABLE` / `TAB_CLOSED` style errors
- `tab_id` remains extension/runtime internal and never becomes model-visible input

Rollout modes in popup:
- `native_only`: force local native host transport
- `dual`: allow manual `native` vs `remote` mode selection
- `remote_preferred`: force remote Butler API transport

Remote mode relay behavior:
- extension starts a background long-poll loop for bind discovery as soon as remote settings are saved
- extension starts a background long-poll loop for `session_key` action dispatch after a bind request is issued
- background relay loops automatically reconfigure when `Remote Butler URL`, token, rollout mode, or session key changes
- extension persists a stable `browser_instance_id` in local extension storage and sends it with remote session/poll/result calls
- Butler delivers pending bind-discovery requests over `/api/v2/extension/single-tab/bind-requests/next`
- extension resolves bind-discovery requests to `/api/v2/extension/single-tab/bind-requests/{dispatch_id}/result`
- Butler delivers pending `single_tab.*` dispatches over `/api/v2/extension/single-tab/actions/next`
- extension posts execution result to `/api/v2/extension/single-tab/actions/{dispatch_id}/result`
- orchestrator rejects dispatch resolve attempts from a different browser instance id

Files:
- `manifest.json` - extension manifest and permissions
- `background.js` - service worker that gathers tabs and talks to the native host
- `popup.html`, `popup.js`, `popup.css` - manual bind-request UI for development

Local development:

1. Enable Developer Mode in Chrome/Chromium/Edge.
2. Load this directory as an unpacked extension.
3. Set `BUTLER_EXTENSION_API_TOKENS` on orchestrator and restart it.
4. In popup keep `Rollout mode = remote_preferred`, then fill `Remote Butler URL` + `Remote API token`.
5. Click `Connect relay` in popup to validate token/access and start long-lived bind relay.
6. Keep extension installed; no `run_id/session_key` is needed for the normal agent-initiated flow.
7. Tell Butler agent to run `single_tab.bind`; extension will receive a tab-discovery request in background.
8. Optional native fallback: install native host manifest for `com.butler.browser_bridge` and run `go run ./apps/browser-bridge`.

Remote mode quick check:

1. Set `BUTLER_EXTENSION_API_TOKENS` on orchestrator.
2. In popup choose `Remote Butler API`.
3. Fill `Remote Butler URL` and `Remote API token`.
4. Trigger `single_tab.bind` from Butler, approve tab selection in Web UI/Telegram, then check active session via remote endpoints.
