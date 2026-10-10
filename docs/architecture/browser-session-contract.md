# Explicit browser execution sessions

Implemented contract for `browser.*` after issue #8. This describes live execution
state, not durable Butler session identity or arbitrary browser crash recovery.
Session Service owns the durable Butler identity, run ordering and execution lease.
Tool Broker remains the only model-call entry point for schema validation, policy,
approval and credential mediation. The browser runtime executes approved calls;
it does not make orchestration decisions.

## Separate independent reads from sequences

`browser.read {url}` creates an isolated temporary context, navigates once, returns
visible text/links/page status and destroys the context. It creates no continuing
capability and accepts no credential material.

`browser.open {url}` creates a context owned by the trusted `ToolCall.run_id`, which
must match the broker-produced `ExecutionContext.run_id`. It returns an opaque
`browser_session_id`, `scope_origin`, `expires_at`, `final_url` and
`observation_version`, alongside the initial observation. The origin is fixed to
the requested HTTP(S) origin and cannot be enlarged by model arguments. The tool
contract's operator-configured domain restrictions also apply. URLs with embedded
credentials or other schemes are refused. Every context is scoped to one run;
there is no transfer or implicit reuse by another run.

The current implementation limits live contexts to 32, with a fixed 15-minute
maximum lifetime. All contexts use isolated, nonpersistent Chromium storage.
Transient browser handles and leases live in the Go runtime's memory; they are
not authoritative durable tasks, runs, memories or Butler sessions. Redis and
PostgreSQL are not used to claim that a live DOM can be restored.

## Operations and effects

| Tool | Effect | Required session guards |
| --- | --- | --- |
| `browser.read` | Navigate in a new temporary context, observe, destroy | Independent URL/domain check |
| `browser.open` | Create a run-owned context and navigate once | Trusted owner, URL/domain check, fixed origin |
| `browser.snapshot` | Observe the current page; **no navigation** | Session ID, owner, expiry, exclusive control, origin/domain |
| `browser.extract_text` | Observe visible selector text; **no navigation** | Same checks as snapshot; selector |
| `browser.navigate` | Navigate the same context inside the fixed origin | Latest version and exact `expected_url`, plus session checks |
| `browser.click` | Click on the current page | Latest version and `expected_url`, plus session checks |
| `browser.fill` / `browser.type` | Change the current input | Latest version and `expected_url`; credential refs for secrets |
| `browser.wait_for` | Wait for visible selector in the current page; **no navigation** | Latest version and `expected_url`, timeout 0–30000 ms |
| `browser.set_cookie` | Install broker-resolved cookie into the same context | Latest version and `expected_url`, typed ref, separate approval, exact cookie host |
| `browser.restore_storage_state` | Install broker-resolved cookies/local storage in the same context | Latest version and `expected_url`, typed ref, separate approval, same origin/host |
| `browser.close` | Destroy the context and private auth state | Trusted owner and exclusive control; no navigation |

Successful effects return a fresh observation and an increasing version. A form
sequence can pass each returned version to its next call without reloading the
page. All calls on a context acquire exclusive control; concurrent calls return
`session_busy`, rather than queueing an effect against a changed observation.
Go checks the owner, expiry, version, observed URL and fixed scope before execution.
The helper also checks the **live** URL and a DOM/input mutation revision just
before an effect. Independent DOM changes therefore cause `stale_observation`.
Take a new snapshot and reassess the intended action; do not blindly replay it.

All page requests, redirects and frame/resource requests are constrained to the
fixed origin; service workers and downloads are disabled. Popups are closed and
do not grant another page capability. Sites requiring another origin, including
cross-origin SSO or CDN resources, may be unavailable in this deliberately narrow
baseline. A separate explicit open inside the user's authorized goal creates a
separate context; it does not share auth or expand the previous context.

## Private credential state

Fill/type accept either ordinary text or `{type:"credential_ref",alias,field}`.
Password inputs require a broker-resolved reference. Cookie and storage-state
arguments accept **only** a typed reference, not raw cookie/storage objects.
Credential Broker checks the observed `expected_url`, tool and autonomy mode;
browser mutations are classified as mutations, not read-only credential use.
The runtime checks the actual page and fixed origin before applying resolved
material. A reference must match the resolved alias and field exactly.

Cookie credentials resolve to JSON with `name`, `value`, `domain`, optional `path`,
`secure`, `http_only`. Storage credentials resolve to JSON with `cookies` and/or
`origins:[{origin,local_storage:[{name,value}]}]`. Cookies must match the session
host; origin entries must match its fixed origin. Cookie path is preserved.
Installed state remains available to later calls in the **same live context**.

Current integration dependency: the existing Credential Broker rejects every
mutating credential use that requires approval, because a verified action-level
approval proof is not yet forwarded after the orchestrator's approval. This is
the separate issue #7; the browser change keeps that safe refusal and does not
invent authorization. Runtime e2e uses trusted resolved-credential fixtures to
prove same-context persistence; Broker e2e proves required approval is refused.
Successful production credential approval must be verified after #7 before
claiming the complete approved-auth flow or closing #8.

Go sends these runtime-only values through a private stdin/stdout JSON-lines
channel to one long-lived Playwright helper. Values are never put in process
arguments. Helper diagnostics and exceptions are not echoed. Logs contain call
identity and tool name only. There are no exported cookies/storage, persistent
browser profiles, screenshot files, or raw DOM/input-value artifacts. Observations
contain visible text and links, with installed credential values, cookie/storage
values and password input values masked if a page reflects them. Known sensitive
URL query fields are masked too. Page observations remain untrusted website data;
this contract does not replace the broader prompt-injection/egress work in #31.

## Close, expiry and failure

`browser.close` destroys storage/context and releases capacity. The helper's
expiry timer closes unused contexts even without another tool call. Go refuses
expired contexts with `session_expired`; later lost/closed IDs return
`session_lost`. Expired metadata is pruned when new contexts are created.

The Go runner keeps one helper generation. Helper/browser crash, process shutdown
or interrupted execution loses all live contexts. Go invalidates all handles
after process failure. A follow-up cannot spawn a fresh process and pretend to
continue; only an explicit `browser.open` or independent `browser.read` can start
one. Interrupted or failed in-flight effects are **not replayed**; their outcome
may be unknown. Failures conservatively invalidate the usable observation, and
the next effect requires another snapshot or an explicit new-session decision.
No guarantee of exact DOM continuation after a crash is made.

`single_tab.*` remains a separate, explicitly approved user-tab capability routed
to `tool-browser-local`. It receives no new list/switch/open/focus capability and
cannot use `browser.*` auth injection. These browser contexts do not access or
automatically rebind a user tab.

## Validation

Unit tests cover owner/context mismatch, version, current URL, origin/domain,
exclusive control, expiry, close capacity, process generation loss, typed auth
resolution and Unicode output. Broker catalog tests reject implicit continuation,
unguarded effects and raw auth objects. Real Chromium e2e exercises a multi-input
form without reloading, private password/cookie/storage persistence and masking,
independent DOM mutation blocking a POST, expiry and process loss.
An additional Broker-to-runtime-to-Chromium e2e loads the real tool catalog,
completes the form sequence and confirms credential approval blocks execution.
Idle expiry cleanup is exercised against the real helper timer too.

Run all checks on the lab with the repository helper and an isolated agent root:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/browser-sessions -Action Sync
pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/browser-sessions -Action Test -Packages ./apps/tool-browser/...
ssh laba 'bash /srv/storage/butler/agents/browser-sessions/current/scripts/dev/lab-browser.sh /srv/storage/butler/agents/browser-sessions'
```

The browser command compiles test binaries in a disposable Go container and runs
them in the existing Playwright image. It does not start/change Compose services
or durable database volumes.
