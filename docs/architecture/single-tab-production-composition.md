# Optional guarded single-tab production composition

This is the typed integration design for the tested component, not an enabled
production configuration. Root application bootstrap/config/registry/Compose
remain the owning integration boundary. No model calls are needed to validate it.

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
Actual PG tests cover this adapter; Web/Telegram delivery, resume, and production
selection/bootstrap wiring remain pending.

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
