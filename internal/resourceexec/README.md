# Guarded controlled-browser resource dispatch

`resourceexec` is an optional boundary between the existing Tool Broker journal,
finite shared budget and `resources.Store`. It protects the controlled
`browser.*` runtime. No production flag, bootstrap, registry or Compose change
enables it. A resource ticket grants no tool, credential, source or model right.

An immutable operator `Profile` pins one stable executor ID, its private runtime
target and exact deployed contract hashes. Browser session IDs come from the
existing runtime; the adapter hashes the profile/session pair and never accepts
a resource ID, host path, mode, lease or fence from model arguments. Credentials
do not partition the resource. All supported operations conservatively use an
exclusive writer lane, including snapshots. `browser.read` and `browser.open`
allocate separate action-bound creation lanes. A successful authenticated open
receipt publishes a private hashed owner/run/task/session mapping; subsequent
operations must match it before spending or claiming. Foreign, invented,
closed, expired and cross-run session handles are denied.

The exact grant's `ClaimGrant` transaction locks live Butler authority, complete
source authority and the session mapping before journal/budget/resource rows.
The post-budget hook acquires and claims the resource, then inserts migration062's
content-free one-use dispatch mapping before the grant/journal CAS. Its deadline
is bounded by the grant, live ownership, actual shared-budget reservation,
browser expiry and the resource maximum of30s. Checks use a fresh PostgreSQL
clock after contended resource/lease locks. Any denial rolls back the whole
outer transaction, including allowances and grant spend. No HTTP/gRPC call or
second pool connection occurs inside these transactions.

`Runtime` wraps the actual browser backend. It requires a dedicated service
token and a private `butler-resource-ticket` metadata value, acquires a local
lane, then calls `Begin` to repeat current authority/source/journal checks and
atomically consume the ticket. This transaction commits before backend
execution. The concrete browser's origin, page-observation CAS, credentials,
network restrictions and cancellation checks still apply. A local fence rejects
old attempts. The in-process lane cache is bounded to1024 distinct resources;
exhaustion denies new lanes rather than dropping fencing state.

`Finish` accepts exact correlated metadata and a response hash, never response
body or credential values. A known successful local lane advances the resource
version and consumes its lease. A completed lane receipt is not recipient
verification: the existing journal still decides `effect_confirmed` versus
`effect_unknown` using the actual tool contract. Cancellation, stale state,
malformed/missing results, failed authority checks, process loss or lost receipts
retain the inflight marker and return an unknown effect. Expiry or restart does
not clear quarantine or replay the action. Independent owner reconciliation
must stop the old executor and provide exact separately validated evidence.

## Explicit trusted composition

All pieces must be installed before advertising an enabled protected catalog:

1. Apply migration062 after058 and the journal/budget/task/source foundations.
2. Construct `resources.New(resources.WorkerGuard{}, ownerReconciliationGuard)`.
   The reconciliation guard must independently validate current owner authority
   and actual evidence. A permissive guard is not an acceptable deployment.
3. Construct `resourceexec.New(pool, resourceStore, sharedBudgetController,
   currentRunSourcesTxGuard, fixedProfile)`. The source callback checks the
   complete actual pinned source graph using this caller transaction.
4. Install the store with `actionauth.PostgresStore.SetDispatchClaimGuard` and
   `broker.Server.SetResourceAdmission`. Wrap the actual router using
   `broker.NewResourceExecutor`; retain existing Broker authentication,
   credential mediation and budget composition.
5. Expose `NewHTTPHandler(store, dedicatedPrivateToken)` only on the private
   service boundary. Construct the runtime client with a fixed internal
   `NewHTTPAuthority` endpoint and the same dedicated token. It uses direct
   connections, rejects redirects and never inherits an ambient provider proxy.
6. Register `NewRuntime(actualBrowserBackend, authorityClient, token)` instead
   of the raw backend. Configure the router's existing target authentication.
   Do not leave an alternate unguarded server reachable for that target.

Constructors/setters are startup composition, not concurrent runtime switches.
Missing pieces deny protected dispatch or leave this adapter unenabled; none
of them creates owner permission. Partial rollout must not advertise readiness.
Dedicated tokens stay in private service configuration, never tools, model
context, queued input, approval text or result JSON. The helper is not a public
owner API, and its authenticated calls cannot provide arbitrary model policy.

## Acceptance and remaining scope

Run [the isolated lab proof](../../scripts/dev/resource-runtime-lab.sh) through
`ssh laba`; it uses a disposable internal network/PG, pool2, fixed synthetic
credentials and the existing Chromium image. See
[acceptance evidence](../../docs/ai/resource-runtime-acceptance.md).

This boundary does not protect legacy `single_tab.*` user Chrome extension
operations. That protocol still needs real user's-tab observation/fencing and
manual-owner-interference acceptance. Production bootstrap/registry/readiness,
workspace/shared-worker callbacks, independent reconciliation and combined
task-tree/memory-conflict scenarios also remain pending for issue17. No model
call, production deployment or claim of complete issue17 acceptance is made by
this module.
