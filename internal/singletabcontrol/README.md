# Guarded owner tab selection

This owning library creates finite owner choices and atomically selected browser
bindings. It does not execute tools, authorize an effect, inspect credentials,
grant account access or reset a resource/budget. PostgreSQL owns durable state.
The optional path remains disabled until the application composes all guards.

`New(pool, Metadata, resources.Guard, resourceexec.SourceGuard, fixedHost)` rejects
missing dependencies. `Request`, `Select` and `Revoke` require an authenticated
`owneridentity.Actor` and a trusted current `resources.Binding`; API/model bodies
must not supply that proof. The runtime callback is `Guard(fixedHost)`.
`WorkerGuard` requires a live worker lease for request and runtime dispatch.
`NewWaiting(pool,metadata,sources,fixedHost)` provides only Select/Revoke on an
existing request while the current task waits for its owner. It resolves the
immutable binding from PG; a verified owner actor, matching run/task revisions,
current mandate/delegation and sources, and released/expired Session lease are
required under the owning lock. A resumed live worker denies this path. It never
creates a token, renews a lease, extends the original finite request expiry or
dispatches work. Metadata runs outside PG; all guards repeat before publication.
The Orchestrator optionally composes waiting-owner choices/select/revoke through its
authenticated Web API. `CheckSchema` pins migration063's enabled triggers and
the additional 064 frozen-choice trigger and actual immutable function before startup.
`Choices` exposes only recorded ordinal, origin, active-at-request and selection
status, repeating current sources before release. Private native refs stay private.
There is no reconstruction of missing old choice metadata or current browser order.
The operator flag defaults false;
stored settings cannot change it. A separate default-false request flag composes
the ordinary Broker request and protected Web owner delivery. Telegram points to
that page and cannot use legacy/generic callbacks. Guarded status/click runtime
composition remains a separate gate.

`RequestContract` has empty public args and only an opaque local request receipt.
`NewRequestClient`/`NewRequestHandler` carry private exact dispatch refs with a
fixed service token; bounded strict JSON, no redirects/proxy and literal-loopback
HTTP/trusted HTTPS keep that token out of models, channels and browser runtimes.
`NewDispatchedRequests` joins the owning dispatch/source/worker guard to atomic
checkpoint publication. Migration065 retains an immutable dispatch mapping.
`CheckSchemaTx` permits fresh pinned-schema checks within the owning transaction
without borrowing another connection. No request receipt authorizes an effect.

`NewDurable` fixes `WorkerGuard` and requires typed `SelectionCheckpoints`.
Pending choice publication and the actual input/run/task suspension commit
together; a lost/missing input claim rolls everything back. `NewWaitingDurable`
requires the same checkpoint adapter and atomically readies that exact input
after selection/revocation. A choice can be revoked while queued. Checks repeat
after checkpoint lock waits; original expiry is unchanged. The owning app
controls recovery using a fresh Session lease and current Catalog pins. Plain
library constructors remain for the independent isolated selection boundary;
their records cannot establish a missing durable input continuation.

Metadata RPCs run outside every transaction. Their fixed client bypasses ambient
proxies/redirects, accepts HTTP only on loopback (remote requires trusted TLS),
bounds timeout/bytes and supports `list`/`check` only. Private actual host/tab IDs
never enter owner choice responses or model tool observations. An owner sees
opaque candidate, origin, ordinal and active marker; these labels grant no rights.

Writers take owning exclusivity before current session/run/task/policy/source
checks, then all sorted candidates, approval, request and selected session.
Claims use owning shared locks before that same selected-domain order. No owning
writer acquires effects/budgets or waits backward for ownership. Final checks
use current PG time after waits, including exact-selection retries.

Migration 063 freezes original owner/run/task/revisions/host/deadline and exact
selected candidate/origin. Triggers prohibit authority mutation, mapping changes
and history deletion. Legacy approval/session records have no guarded mapping and
deny. Revoke disables current selection; it does not clear an uncertain native
lane, reconcile an external effect or resurrect a stopped run's authority.
Expired/stopped binding retirement is a separate authenticated lifecycle control,
not an automatic rebind/cleanup fallback in this library.

The actual two-connection Broker tests cover both claim/revoke commit orders,
exact-choice replay and expiry while waiting on a legacy session row. Real
Chrome tests exercise metadata, atomic owner selection, private Broker/resource
dispatch and an installed Linux native-host manifest. See
[the acceptance report](../../docs/ai/single-tab-resource-acceptance.md) and
[production composition contract](../../docs/architecture/single-tab-production-composition.md).
