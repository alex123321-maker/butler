# Optional guarded user-tab execution

Status: default-disabled implementation and isolated lab acceptance for issue 17.
A tab/session ID, observation, heartbeat or resource lease grants no action,
credential capability or external success verdict. No main registry, bootstrap
or Compose route is enabled by this change.

Exact measured scenarios and remaining acceptance gates are recorded in
[the lab report](../ai/single-tab-resource-acceptance.md).

The first guarded contract supports `single_tab.status` and `single_tab.click`
only, with revision `guarded-1`. The operator pins exact contract hashes, host
identity and private target.
Status requires an explicit approved session; click freezes observation ID and
selector. Legacy revision1 contracts, implicit session fallback, navigation,
keyboard, fill/type, capture, credential refs, arbitrary JS and retry deny.

## Admission

`resourceexec.NewSingleTab` requires the finite budget controller, resource store,
complete source guard and owning selection adapter. `singletabcontrol.Guard`
(`singletab.ResourceGuard` in the application) checks the immutable guarded
selection mapping, actual selected candidate, approved typed selection, active binding,
original run/session, fixed browser instance and trusted selected URL's HTTP(S)
origin in the caller transaction. URL userinfo denies; query/path/fragment never
enter the binding. Native tab metadata and synchronous page origin must both
match that frozen origin. Even a fresh status after cross-origin navigation
denies; same-origin navigation invalidates the previous observation. A
later run cannot inherit it. Selection expires at most 15 minutes from creation,
without extending the original immutable request even if a session expiry is
null. Owner/task/constraints/mandate/source/worker
checks remain independently mandatory.

Owner control first takes the exclusive owning barrier, then current
session/run/task/policy and source locks. It locks the complete sorted candidate
set, approval, immutable request authority/state, then tab session. Broker
preflight takes the shared owning barrier and the same candidate-to-session
order. Irreversible dispatch uses the existing action/effect/budget locks before
its optional post-budget selection/resource hook; runtime checks selection
before its journal and budget/resource checks. Owning controls never acquire
effect or budget locks and never wait backward for ownership while holding a
candidate. Checks after waits reuse held authority and fresh PG time. Failure rolls back all
reservations/grant/resource changes. Migration 063 stores only private binding and
frozen observation refs alongside the one-use ticket, without page/field text,
cookie, password or service token. Downgrade refuses retained history.

## Actual owner selection

`internal/singletabcontrol` owns atomic guarded selection in existing approval/
candidate/session tables plus the immutable request/selection records of 063.
Its constructor requires current transaction-aware execution and source guards;
nil guards deny. Legacy `ActivateFromApproval` rejects its guarded marker and
legacy selection records cannot satisfy its resource guard.

`Controller.Request(ctx, binding)` first verifies authenticated owner and current
execution/source authority, commits that preflight, requests fixed native metadata
outside PG locks, and rechecks authority before atomic pending publication.
The private metadata endpoint supports only `list` and `check`, with bounded
actual host/tab, canonical origin and active marker. It accepts no title, URL
path/query, browser body, cookies, JS, navigation or arbitrary native operation.
The owning response is opaque approval/candidate IDs, origin, generated ordinal
and safe active marker, so two tabs of the same origin remain distinguishable.

`Select(ctx, binding, approvalID, candidateToken)` checks the actual native tab
outside PG locks, then repeats current owner/source checks and atomically marks
exactly one candidate, approves the selection and creates its immutable binding.
Same-choice retries return the same session only after current checks; a different
choice denies. `Revoke(ctx, binding, approvalID)` serializes on the same owning
barrier and disables the selection. These are owner controls, not model tools.
The binding/token and authenticated actor come from trusted owning composition,
never client-supplied authority fields. Metadata grants no tool or account rights.
The measured owning guard requires a live worker lease. Human selection after a
released lease is a distinct current-owner/suspended-run composition and remains
unproven; neither fixture credentials nor a prior lease can stand in for it.

Resource identity derives from operator profile plus actual browser instance and
tab number, not fresh bind-session IDs. Rebinding cannot create a parallel lane.
Changing origins cannot partition the same physical tab into a second lane.
All supported calls use an exclusive lane. The native host exposes authenticated
gRPC around `resourceexec.Runtime` with no legacy HTTP route. The host accepts
only an explicit loopback IP/port; a remote Broker requires an operator-owned
authenticated tunnel or gateway rather than a public/LAN plaintext listener.
Current PG authority
is checked and the ticket consumed before native dispatch; no RPC runs under PG
locks. Deadline is bounded by grant, worker, selection, tool budget and 30s lease.

## Observation and ACK

The isolated main-frame world retains a bounded private DOM/form/focus/scroll
snapshot and random observation ref. Contents never leave that world or enter
storage. DOM records, trusted input/focus/scroll events, changed field properties
and navigation invalidate it and advance version. A new document has no old
observation. Comparison, unique visible hit-target checks, reference consumption
and click execute synchronously in one injected task, with no async gap.

Only resource/fence/dispatch/phase metadata is persisted before scheduling code.
The tab lane excludes concurrent native/relay/legacy execution. Once guarded,
the legacy extension path rejects that tab. Persistent fences survive extension
reload. Missing ACK retains quarantine; restart, expiry, new ticket/session ID
cannot clear it. Host ACK follows successful durable resource Finish. Lost ACK
stops replay. Broker still records an unverified click as `effect_unknown` even
with `dispatch_completed` and a lab recipient request. External verification and
owner reconciliation remain separate.

## Limits and activation gates

Observation covers bounded light DOM, form values, focus and scroll in the main
frame. It cannot prove unchanged hidden JS/server/canvas/shadow-root state or a
recipient outcome. Unsupported actions and oversized documents deny. Host and
browser clocks must be consistent for absolute deadlines. This is not an atomic
transaction across PG/Chrome/recipient; stop after admission may race an already
started effect and retains uncertainty.

User Chrome traffic is not globally intercepted by Butler's controlled-browser
proxy. Durable call/time budget applies, but page-script network privacy/egress
requires separate exact owner policy before activation. No SecretStore values or
cookie export are supplied; private service-token files never enter extension
or model args. Persistent extension metadata is capped at 256 lanes, runtime at 1024;
exhaustion denies. Automatic uncertain-lane cleanup is absent. Future owner
reconciliation must stop the old executor and independently verify its exact
effect before clearing quarantine.

Remaining for this component: production contracts/full startup and real owner
channel composition without raw alternatives, exact account/network policy and
owner effect reconciliation. The isolated Linux proof includes Chrome's actual
installed native-manifest launch and owning selection; human interaction through
the production Web/Telegram channel remains unmeasured. Windows private-file ACL
support is unproven, a platform limitation rather than a Linux release criterion.
Other supported issue17 gates are workspace/runtime callbacks, shared local
weights and memory-conflict admission. Synthetic interference proves the mechanism
and retains truthful uncertain outcomes; it does not finish the whole issue.

Primary references: Chrome [isolated worlds/main-frame injection](https://developer.chrome.com/docs/extensions/reference/api/scripting),
[content isolation](https://developer.chrome.com/docs/extensions/develop/concepts/content-scripts),
[local storage](https://developer.chrome.com/docs/extensions/reference/api/storage),
[MV3 static imports](https://developer.chrome.com/docs/extensions/develop/concepts/service-workers/basics),
and [Playwright extension testing](https://playwright.dev/docs/chrome-extensions).
The actual installed host follows Chrome's
[native messaging manifest/host protocol](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).
