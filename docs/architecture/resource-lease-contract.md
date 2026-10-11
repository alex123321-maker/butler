# Resource arbitration foundation

Status: implementation and lab acceptance in progress for issue17. Migration058
and `internal/resources` are an additive kernel library; production tool/browser
and isolated workspace adapters are not enabled by this change.

Session Service remains responsible for conversation ordering and worker
fencing. Resource arbitration owns finite leases for exact stable kernel IDs
(`workspace:<opaque>`, `browser-tab:<opaque>`, `memory:<opaque>`), immutable human
owner identity, monotonic writer fences and observation versions. It does not
write tasks, sessions, memory, effects or budgets, or resolve credentials.
Owner/context authority must be locked first on the caller transaction; current
task, constraints, parent contract and owner mandate are checked again before
effect publication. No absent-guard or stale-owner fallback exists.

PostgreSQL is the durable source of truth. A complete declared resource set is
sorted lexicographically, limited to16 IDs and granted all or none through a
savepoint. Independent reads share a resource; a write excludes live readers
and writers. Different resources do not require a process/global application
lock. Callers cannot hold one resource while recursively acquiring an unordered
set. Lease admission does not reserve/refill a budget or authorize a Broker
effect. Existing root/mandate/day and delegation budget admission still applies.

An initial lease lasts at most30s and no longer than the current execution
authority deadline. There is no renewal API. Clock/version/authority are checked
after waits and again before use. Unclaimed expired leases cease blocking new
admission, but their old proofs remain unusable. A claimed write installs a
durable inflight marker before external/filesystem execution. Expiry cannot
establish effect absence; that marker blocks both reads and writes until a
confirmed outcome or independent owner reconciliation. Neither cleanup nor
process restart automatically clears it or retries an effect.

Confirmed writes consume their lease and increment the resource observation
version. A plan from the preceding version is denied. Reconciliation requires
authenticated owner authority, evidence validated by the owning effect/artifact
adapter against the exact resource/lease/version, stopping the old executor and
lease expiry. It always advances version and fence, even for a proven absent
effect, and grants no new execution right. History is retained; downgrade refuses
to discard nonempty resource/lease tables.

The resource observation version is separate from workspace file version/hash,
browser page observation version and owner-memory candidate revision. Those
owning mechanisms retain their existing CAS/review rules. File adapters must
record drift/unknown when filesystem effects occur but PostgreSQL persistence
fails. Browser adapters must detect user interference. Memory facts still enter
candidate/review queues; a lease cannot replace owner acceptance of preferences.

Pending issue17 acceptance: production Broker/runtime wiring, actual workspace
file conflicts, one user-tab executor with manual interference, task-tree stop
of isolated executors and the combined root budget/memory-candidate scenarios.

## Current lab evidence

Read-only source snapshot `20261011-071207-877` on `ssh laba` passed
`go test -race -p1 -count=1 ./internal/resources` with actual isolated PostgreSQL
and pool2. All six top-level tests executed, including four authority-change
subtests and a contended row-lock/deadline case. Earlier failure was in the test
observer's cached PostgreSQL activity snapshot; refreshing that snapshot made
the observer follow actual row-lock waiting. It did not alter production policy.
This demonstrates the library boundaries, not completed issue17 acceptance.
