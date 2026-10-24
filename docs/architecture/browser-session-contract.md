# Explicit browser execution sessions

Credential-aware fill/type now return optional `credential_verified`: a private
runtime equality check against the exact value resolved by Broker for that
operation. No value, hash, length or character feedback is returned. A failed
comparison returns `credential_mismatch`. This is momentary local field evidence;
remote submission and successful authentication remain separate outcomes.

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
`observation_version`, alongside the initial observation. `scope_origin` reports the current page origin, not a navigation boundary.
Revision 3 permits public HTTP(S) origins, redirects and CDN resources in the
same isolated context. Operator-configured domain caps still apply to every request. URLs with embedded
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
| `browser.open` | Create a run-owned context and navigate once | Trusted owner, public URL/domain checks |
| `browser.snapshot` | Observe the current page; **no navigation** | Session ID, owner, expiry, exclusive control, current domain |
| `browser.extract_text` | Observe visible selector text; **no navigation** | Same checks as snapshot; selector |
| `browser.navigate` | Navigate the same context across permitted public origins | Latest version and exact `expected_url`, plus session checks |
| `browser.click` | Click on the current page | Latest version and `expected_url`, plus session checks |
| `browser.fill` / `browser.type` | Change the current input | Latest version and `expected_url`; credential refs for secrets |
| `browser.prepare_fill` / `browser.prepare_type` | Change input locally with all network frozen persistently | Same latest observation/URL/owner checks; exact approval or current mandate; credential refs for secrets |
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

### Bounded local preparation and truthful external outcomes

For a multi-step form use `open -> prepare_fill/prepare_type -> snapshot/wait ->
separately authorized click`. Preparation freezes **all** page requests before
input events run. The Go proxy cancels in-flight HTTP and closes established
CONNECT tunnels; the reviewed helper also aborts every route. Immediate and
delayed GET/POST autosave cannot release the new draft. Snapshot, extraction,
waiting, cookie/storage operations and later preparations never thaw it.
Navigation and legacy live fill/type in a frozen context return `network_frozen`.
Close/open creates a new empty context; it cannot restore a private draft.

Only a separately authorized click opens a bounded network window, clipped to
the trusted dispatch deadline and context expiry. The helper keeps its route
frozen until the live DOM/version/URL guards pass. Stale clicks therefore cannot
thaw a prepared draft. Success or failure freezes the context again; expiry,
cancellation or process loss closes connections. A click permits the site's
opaque effects inside that permitted public network window, including callbacks; it is not proof
that one particular remote effect occurred.

Revision 3 `browser.read/open/navigate` contracts classify their enforced
GET/HEAD observation as `read` (`read_only/read_result`). This is the same method
boundary as HTTP reads, not a claim that an arbitrary application cannot mutate
state on GET. Broker rejects older navigation contracts instead of granting
them the new guarantee. Privacy, resource and approval checks still apply.

The new preparation tools use revision 2 `prepare` contracts and
`local_browser_preparation_v1`. Their response contains `local_preparation:true`,
`network_state:"frozen"`, session ID, exact observed URL and a newer observation
version. Private journal policy pins the original session/URL/version; missing,
mismatched, failed or truncated proof stays unknown. Confirmed evidence means
**only local prepared browser state**, not remote submission. Local preparation
has no automatic retry/crash-recovery guarantee. Existing live fill/type and
click stay opaque: a DOM acknowledgement without an independent recipient
receipt becomes `effect_unknown`, and the durable unknown-effect guard blocks
later effects. Approval is execution authority, not retrospective evidence.

All page requests, redirects and HTTP(S) frame/resource requests pass the private
multi-origin proxy and the initial contract's domain caps. Checked DNS is pinned
to public addresses for each new connection. Literal/private destinations and
mixed public/private DNS remain denied. Native Chromium same-origin, cookie and
TLS rules remain enabled. Service workers, WebSockets, downloads and extra popup
pages remain disabled; popup-only SSO is not supported. Multi-origin navigation
does not share another run's browser context or grant additional credentials.

## Private credential state

Live and local-preparation fill/type accept ordinary text or `{type:"credential_ref",alias,field}`.
Password inputs require a broker-resolved reference. Cookie and storage-state
arguments accept **only** a typed reference, not raw cookie/storage objects.
Credential Broker checks the observed `expected_url`, tool and autonomy mode;
browser mutations are classified as mutations, not read-only credential use.
The runtime checks the actual current page origin before applying resolved
material. A reference must match the resolved alias and field exactly.

Cookie credentials resolve to JSON with `name`, `value`, `domain`, optional `path`,
`secure`, `http_only`. Storage credentials resolve to JSON with `cookies` and/or
`origins:[{origin,local_storage:[{name,value}]}]`. Cookies must match the session
current host; origin entries must match the current page origin. Cookie path is preserved.
Installed state remains available to later calls in the **same live context**.

Credential use follows the [exact action authorization contract](action-authorization-contract.md).
An owner decision binds alias/field, resource, arguments, tool and task versions.
Broker verifies it before secret release; runtime verifies its actual page
before injection. Unapproved/stale decisions fail closed. Real Broker/Approval
Service/Chromium e2e verifies successful approved password use, form continuity,
secret masking and refusal before approval. User-tab approval remains separate.

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

`single_tab.*` is disabled in the standard registry. Its optional legacy capability is routed
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
completes the form sequence and confirms unapproved credentials block execution.
A further e2e starts the actual Broker process, real PostgreSQL approval and
credential services, and the authenticated private authority endpoint; it
completes an approved password/form flow once and rejects changed args/replay.
Idle expiry cleanup is exercised against the real helper timer too.

The local-preparation Chromium fixture checks immediate and delayed GET/POST
autosave, preserved freeze across snapshot/extract/wait, stale Go and live-DOM
click guards, masked password reflections, exactly one intended form POST and
post-click callback suppression. Proxy fixtures verify HTTP cancellation,
established CONNECT shutdown, no DNS/dial after freeze and automatic window
expiry. Real PostgreSQL journal/budget tests permit read plus two local
preparations, retain the final click as unknown across restart, and reject a
follow-up without spending another reservation. These are synthetic public-DNS
fixtures in disposable containers, not real external writes.

Run all checks on the lab with the repository helper and an isolated agent root:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/browser-sessions -Action Sync
pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/browser-sessions -Action Test -Packages ./apps/tool-browser/...
ssh laba 'bash /srv/storage/butler/agents/browser-sessions/current/scripts/dev/lab-browser.sh /srv/storage/butler/agents/browser-sessions'
```

The browser command compiles test binaries in a disposable Go container and runs
them in the existing Playwright image. It does not start/change Compose services
or production database tables. Approval e2e uses disposable schemas in the
separate `butler_actionauth_test` database.

## Owner viewing and manual control

`/browser` opens the browser panel on the continuous Mind Web surface, also
available from its top browser icon. It displays 1280x720 Chromium page frames
using bounded JPEG polling, with no separate legacy route or layout.
It is the server's page viewport, not the user's laptop or a native Chrome
window. The existing owner-control bearer authenticates the public
`POST /api/v4/server-browser` before its body is parsed. Core supplies immutable
owner identity and forwards to Broker over its private action-service channel.
Broker validates the stored successful-open mapping, live core run/task/lease,
and owner for every frame or input command. Releasing an existing controller is
cleanup: it still checks authenticated owner, mapping, controller hash and epoch,
but remains possible after task cancellation or lease expiry. This permits no new
viewing/input or agent authority. A dedicated runtime-only token protects
Broker -> browser HTTP; runtime receives no core, owner or PostgreSQL credential.
Both private clients reject redirects and ambient proxy settings.
Core's layered startup loader preserves `BUTLER_BROWSER_CONTROL_URL` from the
operator environment only; DB/UI settings cannot enable or redirect this private
authenticated route. Missing Broker service proof or an invalid endpoint path,
userinfo, query or fragment prevents startup instead of presenting a working UI
with no owner controller.

Migration086 stores run/owner/task mapping, finite expiry, control mode,
monotonic epoch and controller hash. It stores content-free operation/ACK audit
rows only: no screenshot, URL, keyboard text, cookies or passwords. The actual
Chromium context remains transient and expires after 15 minutes. Registration
occurs only on a correlated schema-valid successful Broker `browser.open`.
This optional composition supports workspace tools alongside independent server
contexts; it is separate from guarded shared user-tab resource composition.

Takeover commits the durable owner fence before contacting runtime. The runtime
serializes human and agent requests with the same session mutex and checks the
controller and epoch. Explicit takeover can replace a lost browser-page
controller; old input/release commands cannot cross the new fence. Taking control
resumes the site's background traffic within the existing public-address,
domain, byte and expiry limits, including a previously frozen page. Agent
browser actions are blocked before grant claim/budget spend. Core waits between
model turns, proposals and received provider events while owner mode is active.
An already started inference cannot be undone; normal run cancellation, lease
renewal and budget deadlines remain in force.

Broker pins each admitted session call to the current control epoch in private
gRPC metadata. With the owner channel enabled, runtime requires exactly that
epoch under its session mutex, including reads and close. Release advances the
runtime epoch before ACK. Delayed calls from before takeover cannot execute
after return. A late snapshot/extract result can clear `fresh_needed` only in
the exact epoch admitted by Broker. An operation already executing can finish
before takeover ACK; no human input is acknowledged concurrently with it.

Manual click, double click, scroll, key, text and navigation commands use a bounded
private protocol. Each input consumes its frame token and validates live document
identity/revision of every main and child document against the captured frame,
including cross-origin iframe replacement, navigation, DOM changes and scrolling.
Playwright's screenshot caret
mutation is disabled so taking a frame does not invalidate itself. Animated or
changed pages can require another frame. Input is never automatically replayed;
a lost command/ACK is reported as uncertain. Human input is independent direct
owner activity, not an agent approval, action mandate, task result or proof of
any recipient effect. The audit's acknowledged status means local runtime ACK.

Release stops in-flight proxy traffic, disables page writes, restores ordinary
GET/HEAD viewing and invalidates all preceding agent observations. Broker keeps
the run blocked until release ACK and durable mode update both succeed. A fresh
snapshot/extract is required before another guarded agent action. Lost release
ACK keeps the durable fence closed; recovery requires explicit takeover/release.
Closing/reloading the viewer does not silently release control. Session expiry
or runtime loss never recreates or replays the browser.

Frames, the actual current page URL and owner input are returned only to the authenticated viewer. Ordinary
model results explicitly discard the private frame field. Typed text, password
inputs, installed credentials and active input values are retained only inside
the helper's redaction set; subsequent visible observations mask reflected values.
Overlapping values and successive keyboard prefixes are masked against original
text so a shorter match cannot prevent masking the complete longer value.
The Web page keeps proof/controller/input/frame in memory and clears them when
proof changes or the component unmounts; it uses no local/session storage.
Any failed control request discards queued keyboard input; subsequent keys are
not sent against the uncertain result of an earlier command.

Compose the owner channel explicitly with `deploy/docker-compose.server-browser.yml`,
a dedicated browser-control token file, existing owner authentication and migration086.
No browser HTTP/CDP/VNC ports are published. The shared lab stack is not switched
by the validation script; see `scripts/dev/server-browser-lab.sh`.
