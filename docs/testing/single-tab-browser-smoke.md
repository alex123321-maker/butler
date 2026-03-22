# Single-Tab Browser Smoke

Purpose:
- verify the end-to-end single-tab browser flow with a real Chromium-based browser
- verify `BROWSER_TAB_SELECTION` approval delivery, session activation, bound-tab actions, and screenshot artifact persistence

Prerequisites:
- copy `.env.codex-windows.example` or `.env.example` to `.env`
- start backend services with `make up` or the hybrid host workflow used for Butler development
- run `browser-bridge` and `tool-browser-local` on the host
- load `web/extensions/chromium-butler` as an unpacked extension in Chrome/Chromium/Edge
- install the native messaging host with:

```powershell
pwsh -NoProfile -File scripts/dev/install-browser-bridge-host.ps1 -Browser Chrome -ExtensionId <extension_id>
```

- if Telegram approval is part of the test, configure:
  - `BUTLER_TELEGRAM_BOT_TOKEN`
  - `BUTLER_TELEGRAM_ALLOWED_CHAT_IDS`

Recommended host-side processes:

```powershell
go run ./apps/orchestrator
go run ./apps/browser-bridge
go run ./apps/tool-browser-local
```

Manual smoke path:
1. Open a few ordinary HTTP(S) tabs in the same Chromium profile.
2. Open the Butler extension popup.
3. Enter a real `run_id` and `session_key`, then create a bind request.
4. Confirm a `browser_tab_selection` approval appears in Butler Web UI and, if configured, in Telegram.
5. Select exactly one tab.
6. Verify the chosen approval returns an active `single_tab_session`.
7. Trigger representative actions through Butler:
   - `single_tab.status`
   - `single_tab.navigate`
   - `single_tab.click`
   - `single_tab.fill` or `single_tab.type`
   - `single_tab.wait_for`
   - `single_tab.capture_visible`
8. Confirm `capture_visible` returns an artifact-backed `image_ref`, not an inline data URL.
9. Close the bound tab and confirm the next action fails with `TAB_CLOSED`.

HTTP/API verification:

1. Find pending or recent browser approvals:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/v2/approvals
```

2. Read the active single-tab session:

```powershell
Invoke-RestMethod "http://127.0.0.1:8080/api/v2/single-tab/session?session_key=<session_key>"
```

3. Inspect the task artifacts after `capture_visible`:

```powershell
Invoke-RestMethod "http://127.0.0.1:8080/api/v2/tasks/<run_id>/artifacts"
```

Expected results:
- bind request creates a durable `browser_tab_selection` approval
- exactly one selected tab yields an `ACTIVE` `single_tab_session`
- actions execute only in the selected tab
- cross-domain navigation inside the same tab keeps the session active
- `capture_visible` creates a `browser_capture` artifact and returns its `artifact_id` as `image_ref`
- closing the bound tab moves the session to `TAB_CLOSED`

Failure signals:
- extension popup cannot reach `com.butler.browser_bridge`
- no approval appears in Butler Web UI or Telegram
- session never becomes `ACTIVE`
- actions fail while the tab is still open and selected
- `capture_visible` still returns a `data:image/...` payload instead of an artifact id
- closing the tab does not invalidate the session
