# Chromium Butler Extension

Manifest V3 scaffold for the Butler single-tab browser bridge flow.

Current responsibilities:
- enumerates HTTP(S) tabs from the local Chromium-based browser
- sends `bind.request` messages to the host-installed `browser-bridge` native messaging companion
- checks the currently active `single_tab_session` for a Butler session key
- executes host-routed `single_tab.navigate`, `reload`, `go_back`, `go_forward`, `click`, `fill`, `type`, `press_keys`, `scroll`, `wait_for`, `extract_text`, and `capture_visible` requests inside the bound tab
- provides a minimal popup UI for manual development testing

Current limitations:
- native messaging host installation remains manual
- approval selection still happens in Butler web UI or Telegram, not inside the extension
- remote transport mode is not production-ready yet; current default is native host mode

Notes:
- `capture_visible` returns a screenshot to Butler, and the host runtime persists it as a durable `browser_capture` artifact before returning the final `image_ref`.

Expected native messaging host:
- `com.butler.browser_bridge`

Target connection modes:
- `native` (current): extension <-> native messaging host (`browser-bridge`) <-> Butler API
- `remote` (planned): extension <-> Butler API (HTTPS + extension auth) with server-side relay for action dispatch

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
- extension starts a background long-poll loop for `session_key` when remote bind/session calls are used
- extension persists a stable `browser_instance_id` in local extension storage and sends it with remote session/poll/result calls
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
3. Install a native messaging host manifest for `com.butler.browser_bridge`.
4. Run `go run ./apps/browser-bridge` and start Butler orchestrator.
5. Open the popup, enter `run_id` and `session_key`, then create a bind request.

Remote mode quick check:

1. Set `BUTLER_EXTENSION_API_TOKENS` on orchestrator.
2. In popup choose `Remote Butler API`.
3. Fill `Remote Butler URL` and `Remote API token`.
4. Create bind request and check active session via remote endpoints.
