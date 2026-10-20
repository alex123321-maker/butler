# Resource arbitration foundation

Workspace owner reconciliation now has one concrete discard-only composition:
[explicit owner discard](isolated-code-workspaces.md#explicit-owner-discard).
An immutable authenticated owner decision, matching actual old executor inode,
per-copy drain, terminal tree, exact current resource/lease/CAS and natural expiry
precede physical erasure. An independent synced absence receipt satisfies the
private guard, advancing version/fence while retaining original UNKNOWN effect
and spent budget. Neither owner Actor alone nor a model/RPC receipt string can
reconcile. Lost erase acknowledgment permits observation only; another effect
needs another explicit owner decision. No new lease or worker is minted.

Status: the resource, guarded-tab and workspace adapters are implemented for the
supported Linux runtime. The [current-main local acceptance](../testing/issue-17-current-main-local.md)
maps issue17's eight criteria to owning PostgreSQL, Broker, filesystem and actual
Chromium checks. Operator enablement remains separate from kernel acceptance.
Migration058 and `internal/resources` are an additive kernel library; production
tool/browser and isolated workspace adapters default disabled. Optional guarded single-tab
bootstrap is specified in [production composition](single-tab-production-composition.md);
it joins the actual shared budget and permanent action claim on the same transaction.

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

The local issue17 acceptance entrypoint exercises production Broker/runtime
wiring, actual workspace conflicts and owner discard, one tab executor with
manual interference, task-tree stop and shared root-budget/candidate admission.
It uses synthetic resources without a personal browser profile or model calls.

## Optional controlled-browser dispatch boundary

Migration062 and `internal/resourceexec` add a tested, default-unenabled adapter
for the controlled `browser.*` service. They do not change legacy `single_tab.*`
or deploy a new production route. Stable profile/contract/runtime identity is
operator-owned. Browser resources derive from runtime-created opaque session
handles; a private successful-open mapping binds their hash to the actual
owner/run/task. Foreign or fabricated handles cannot claim or spend a resource.
No model resource IDs, host paths, modes, fences or credential values are used.
All current browser operations serialize on an exclusive lane; different
creation actions have distinct bounded lanes.

Admission order is live ownership, session/run/task and current assigned policy,
complete source authority, private browser mapping, permanent effect, finite
shared budget, sorted resource/lease rows and dispatch mapping. The complete
authority/source set is stabilized before the budget/resource locks; repeated
checks use only those already held authority rows. The optional journal hook
reserves/claims the resource in the same transaction as budget and grant spend.
After resource waits, fresh PostgreSQL time must remain within the grant,
ownership, task/policy, browser and actual budget deadlines. Whole admission
rolls back on conflict or expiry, without partial grant/counter changes.

Runtime online admission takes the same authority/source/browser locks before
effect and grant/intent/dispatch locks, then the existing resource lease. It
matches outcome recording's effect-before-grant order. Browser-close receipt
locks its mapping for update before journal/resource rows to avoid a late lock
upgrade. A local lane is acquired before the one-use ticket CAS. Broker claim
commits before its runtime RPC; authority admission commits before backend
execution. Neither callback issues an RPC while holding PostgreSQL locks. The private authority endpoint
uses a dedicated service token, a fixed direct internal HTTP(S) destination,
bounded messages and no redirects or ambient provider proxy.

The concrete backend retains origin/credential/page CAS checks. Its bounded
context propagates cancellation, but dispatch already committed before a later
owner stop may have begun an external effect. Such uncertainty is preserved.
Receipts contain correlated IDs and a result hash only. A successful local lane
consumes its lease and advances the resource version without asserting external
recipient verification. Missing/stale/lost receipts retain durable quarantine;
no expiry/restart replay or automatic reconciliation exists. Browser sessions
remain run-owned and cannot be handed to another task by this adapter.

See [composition and limitations](../../internal/resourceexec/README.md) and
[actual PG/Broker/Chromium evidence](../ai/resource-runtime-acceptance.md).

Migration063 adds the separately disabled [guarded user-tab boundary](single-tab-resource-boundary.md).
Current selection, authority and finite budget remain necessary. Native-host and
extension CAS/ACK quarantine supplement arbitration; real user/channel acceptance
and personal-account activation remain separate from the local kernel checks.

## Historical foundation evidence

Read-only source snapshot `20261011-071207-877` on `ssh laba` passed
`go test -race -p1 -count=1 ./internal/resources` with actual isolated PostgreSQL
and pool2. All six top-level tests executed, including four authority-change
subtests and a contended row-lock/deadline case. Earlier failure was in the test
observer's cached PostgreSQL activity snapshot; refreshing that snapshot made
the observer follow actual row-lock waiting. It did not alter production policy.
This demonstrates the library boundaries, not completed issue17 acceptance.

The workspace adapter now requires exact current action/source admission plus
independent actual namespace/file evidence. Create and Write use committed claims,
resource update locks from their first check, and same-TX confirmation/consumption.
The [combined audit](../ai/workspace-resource-acceptance.md) proves actual resource
lease and file metadata behavior on PG/pool2; production Broker/runtime callbacks
and full issue17 scenarios were pending in that historical snapshot. The current
local receipt above exercises the implemented production composition.
