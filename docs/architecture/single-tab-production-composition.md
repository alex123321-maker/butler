# Optional guarded single-tab production composition

Root application bootstrap now composes optional waiting-owner decisions behind
an operator-only, default-false flag. A separate default-false request flag now
composes ordinary worker request creation, exact Broker control and owner delivery.
Private guarded status/click runtime composition remains pending.
Durable selection/checkpoint queueing and fresh core recovery are composed under
the same optional flag.
No model calls are needed to validate this owner boundary.

## Operator and owning setup

Default is disabled. Pin a fixed resource profile ID, actual installed browser
instance, exact deployed `guarded-1` status/click contract hashes, private runtime
target, resource-authority endpoint, metadata endpoint and private service-token
file. These values are operator composition, never model or channel request args.
Do not advertise readiness from metadata availability alone. Startup must prove
063 current-schema tables and enabled immutable triggers, complete current
task/source/budget hooks, private authentication and the deployed exact hashes.

Construct `singletabcontrol.New(pool, metadata, resources.WorkerGuard{}, sources,
fixedBrowserInstance)` in owning orchestration. `sources` is the complete current
Catalog/run guard using the same `pgx.Tx`; it may not borrow another connection
or RPC. For owner controls, resolve a current live run/task/session token and
finite deadline from trusted PG/Session Service, then attach verified channel
`owneridentity.Actor`. Never decode a `resources.Binding` or owner proof from the
request body. Source/catalog access and account policy remain independently checked.

The authenticated owning API uses only these public shapes:

- Request: current owned run ref; server resolves the binding. Return
  `Selection{approval_id, candidates:[{candidate_token,origin,ordinal,active}],expires_at}`.
- Select: opaque approval ID and candidate token. Call `Controller.Select` and
  return the approved opaque session ID. No client host/tab/URL/expiry overrides.
- Revoke: opaque approval ID. Call current `Controller.Revoke`; owner Stop/revoke
  must still fence execution. Disabling the feature does not stop an accepted
  dispatch or reconcile an uncertain lane. Expired/stopped record retirement must
  be a separate owning control, never a retry or permission extension.

Use the existing owner Web/Telegram approval delivery surface with sanitized
choices. Do not route guarded approvals through legacy non-atomic selection.
The library/Chrome dispatch path requires a live worker lease. The separate
`NewWaiting` owner adapter permits only selection/revocation of an existing
finite request after Session Service releases the worker lease. It resolves
bindings from PG, requires a verified channel actor, current matching
waiting-for-user task and awaiting-approval run, current authority and sources,
and no live executor. It repeats those checks under the owning barrier after
metadata RPC. Original expiry is never extended; elapsed requests deny. Its
LeaseEpoch=0 binding confers no execution authority. Resuming execution must
acquire a new Session lease and pass ordinary WorkerGuard/Broker checks.
Actual PG tests cover this adapter. `NewDurable` composes the owning input
checkpoint hook, publishing finite choices, run/task waiting state and actual
claimed input suspension in one transaction. The bootstrap uses
`NewWaitingDurable`: select/revoke queues that exact stored continuation, including
when an earlier decision precedes the old worker's finish. Old requests without
the accepted-input selection marker deny. Core `Resume` acquires a new Session
lease, reloads current definitions, validates the actual input claim and current
Catalog/task/guarded selection, then emits the opaque selection as private DATA
into a fresh provider context. Original deadline stays finite; advanced tool
checkpoints cannot replay an effect through this path. Request creation through
the ordinary worker and owner delivery is described below; guarded status/click
production runtime wiring remains pending.
The Web decision API itself is composed
at `POST /api/v3/tab-choices/{approval_id}/select` (`candidate_token` only) and
`/revoke` (`{}` only), with mandatory configured Bearer owner proof. Unknown,
duplicate, null, case-alias and oversized request fields deny. The only return
from selection is an opaque session ID, with no execution or resume verdict.
No private metadata or library error text enters responses. The startup-only
`BUTLER_GUARDED_TAB_OWNER_ENABLED` requires fixed `BUTLER_GUARDED_TAB_HOST`,
`BUTLER_GUARDED_TAB_METADATA_URL`, `BUTLER_GUARDED_TAB_TOKEN_FILE` and owner auth.
Database settings cannot enable it or change the authenticated destination.
Startup requires the 063 dispatch column, all three enabled immutable triggers and
the pinned actual function body; missing or replaced safeguards prevent startup.
The metadata client retains loopback-only HTTP / trusted HTTPS, bounded direct
requests without ambient proxy or redirects. No runtime-ready assertion follows
from composing this endpoint.

Migration064 retains immutable sanitized choice metadata: original ordinal and
active-at-request boolean alongside the opaque candidate and canonical origin.
It records no private native reference or page body and denies dropping retained
history. `GET /api/v3/tab-choices/{approval_id}` requires current waiting-owner
authority and sources under the same owning transaction and exposes these fixed
fields plus selected state. It does not contact Chrome or claim a current active
tab. Requests predating this metadata have no reconstructed view. The Approvals
page uses this protected read and the existing select/revoke route; generic
approval cannot substitute for the guarded choice. Owner proof stays only in
page memory; proof changes fence late responses. Executing worker request
creation and owner delivery have a separate operator composition below. The
actual human channel workflow and private status/click runtime composition remain
independent release gates.

## Ordinary worker request composition

`BUTLER_GUARDED_TAB_REQUEST_ENABLED` defaults false in both owning orchestrator
and Broker. Orchestrator requires owner controls, private action authority and
063–065 pinned immutable schema. Broker requires PG/private authentication and
fixed `BUTLER_GUARDED_TAB_REQUEST_CONTROL_URL` at `/internal/guarded-tab-request`.
HTTP is restricted to literal loopback; remote service topology requires trusted
HTTPS/tunneling. No model/settings/channel body selects a private destination.

Broker registers the exact empty-args `single_tab.bind.guarded` revision1 local
control contract and denies Chrome/legacy routing for it. Scoped catalog and
prepare require owning readiness. Existing autonomy/action approval, complete
source/task checks and shared budget precede permanent dispatch. The private
grant pins a hash of the actual current ingress claim, independently of immutable
owner-approved intent: a new authorized worker may have a new claim, while an
old dispatch cannot borrow it. Raw claim IDs are not added to public tool args or
owner approval bindings. Owning request reads the permanent claimed journal,
derives the actual Session fence/owner/channel/input, and repeats those guards
in the same transaction before and after native metadata collection outside PG.
No request RPC can select/revoke, execute Chrome, fetch a credential or resume.

Migration065 atomically maps that request to its exact run/call/effect/grant/hash;
finite choices, real claimed-input continuation, run awaiting approval and task
waiting state commit with it. Core finds this durable suspension before ordinary
tool transcript/checkpoint/provider continuation, even after a lost private ACK.
It never retries the request. Web reads the durable protected choice; Telegram
sends a fixed direction to Butler's Approvals page without legacy/generic
callbacks. Delivery failure leaves the durable request available. Fresh recovery
also checks that a mapped request's journal is confirmed: owner selection cannot
settle a lost/uncertain Broker outcome. This route grants no status/click action
authority or claim of browser/recipient/model success.

## Broker and runtime composition

Construct `resourceexec.NewSingleTab(pool, resourceStore, sharedBudgets, sources,
singletabcontrol.Guard(fixedBrowserInstance), fixedProfile)` using the same owned
PG pool. The existing `actionauth.PostgresStore.SetDispatchClaimGuard` receives
this post-budget guard; Broker gets `SetResourceAdmission` and
`NewResourceExecutor(store, authenticatedRouter)`. The resource authority handler
is `resourceexec.NewHTTPHandler(store, privateToken)` on a private service route.

Only exact `single_tab.status` and `.click` guarded-1 contracts may route to the
guarded target. Replace their raw route entirely. An existing controlled-browser
profile is a distinct explicit store/route; if both are composed, use an exact
server-owned namespace/target dispatcher with no implicit fallback. Unconfigured
protected tools deny before secret release/dispatch. The finite shared controller
reserves actual calls and never refunds uncertain effects. Resource tickets stay
in private runtime metadata, not approvals, tool args or model observations.

The guarded host command accepts these operator-only references:

```text
BUTLER_GUARDED_BROWSER_TOKEN_FILE=<private regular mode0600 file>
BUTLER_GUARDED_BROWSER_AUTHORITY_URL=<fixed authenticated private endpoint>
BUTLER_GUARDED_BROWSER_GRPC_ADDR=127.0.0.1:19090
BUTLER_GUARDED_BROWSER_METADATA_ADDR=127.0.0.1:19091
```

The optional metadata listener only implements fixed authenticated list/check.
The native host has no raw legacy dispatch/bind HTTP endpoint. Its explicit
loopback IPv4/IPv6 listener rejects wildcard, DNS and LAN binds. An operator-owned
authenticated tunnel/TLS gateway is required for remote Broker reachability;
do not transmit a bearer token across an open LAN. The composition proof must
cover that exact route; the isolated fixture does not claim a remote tunnel.

Install the Linux native-host manifest for the exact extension origin and
absolute host executable, with a private launcher/token path. Chrome launches
the host via `connectNative`. Optional Compose can own the private authority and
tunnel/reference, mounting only the token file read-only and publishing no open
listener. Browser/extension/model never receive the token. Windows ACL support
is unproven and is a separate platform boundary, not a Linux acceptance criterion.

## Activation acceptance

Repeat actual owner selection and same-origin duplicate choices through the
composed owner channel, then actual private Broker status/action. Confirm current
owner/task/source/approval/deadline/fence, manual/DOM/navigation staleness,
claim/revoke order, dual executor exclusion, uncertain ACK/restart quarantine and
absence of all raw alternative routes. This component does not intercept every
network request from the user's page; exact owner account/network policy is still
required. A local native ACK is not verified recipient success.

Whole issue17 also needs the already reserved workspace/runtime, shared local
weight and memory-conflict adapters. These production integrations remain
separate explicit gates; the component's isolated proof does not close them.
