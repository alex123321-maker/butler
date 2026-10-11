# Finite execution resource leases

`resources` owns migration058's PostgreSQL resource/lease rows. It receives exact
kernel resource IDs, private current worker bindings and a caller transaction.
It adds no dependency and reads task/session authority through `WorkerGuard`.
Both execution and independently evidenced owner reconciliation guards are
required; absent guards deny construction. A resource lease grants no tool,
credential, budget or model permission.

Register resources through their owning kernel adapter before admission.
`AcquireTx` sorts up to16 resources, admits the complete set with a savepoint,
permits concurrent reads and excludes conflicting writes. Leases last at most30s
and are bounded by live execution authority; there is no renewal API. Observation
version and writer fence are checked after row-lock waits and at every use.

Commit `ClaimTx` before starting a write whose effect may outlive a transaction.
The durable inflight marker survives process loss/expiry. Confirmed outcomes
consume the lease and advance the resource observation version. An uncertain
write blocks a successor until an authenticated owner provides independent exact
effect/artifact evidence. Reconciliation invalidates old plans and grants no
new lease. It must occur after stopping the old executor and its lease expiry.

All calls use the same caller transaction with authority locks before ordered
resource locks. A failed acquisition rolls back partial leases/fences even if
the outer transaction commits. Callers must roll back other failed mutations.
Never acquire a new resource after already holding a different unsorted set;
declare the complete required set at admission. Release transactions touch one
resource, or callers process their set in the same order.

Lab PostgreSQL/race tests exercise pool2, separate Store instances/session
owners, read sharing/exclusive writes, opposite-order complete sets, no partial
admission, expired writer fencing, current owner/task/parent revisions,
independent reconciliation, confirmed version changes and expiry during row
lock waiting. Broker/browser/workspace production composition remains pending;
this library does not enable parallel effects or complete issue17 on its own.

See [resource contract](../../docs/architecture/resource-lease-contract.md).

The additive [controlled-browser adapter](../resourceexec/README.md) now joins
real Broker grants/budgets and guarded gRPC runtime admission on isolated PG and
Chromium. It conservatively serializes controlled browser calls and keeps lost
outcomes quarantined. Production composition, user's `single_tab` fencing,
workspace/shared-worker wiring and complete issue17 acceptance remain pending.
