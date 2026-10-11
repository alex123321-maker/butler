# Durable bounded delegation

Implementation in progress for issue16. Production/model acceptance remains
pending. Optional operator composition registers fixed context-only research
admission, using existing input workers; no new owner endpoint is exposed.
The shared lab deployment has not enabled this feature.

Session Service continues to own independent execution identities and ordering.
Task Service owns child tasks and durable contracts. Orchestrator owns child
admission/scheduling and result collection; Tool Broker remains the sole model
tool validation, authorization and routing boundary. Runtime processes cannot
create authority or spawn independent orchestration.

Migration057 records immutable contract identity, the exact parent task and
constraints revisions, root task, source reference, permission set, finite budget,
deadline, depth and dependency records. Free-form candidate instructions belong
in versioned Catalog data with complete parent edges, never in credential state
or a model-authored owner policy. A model result is data and remains unverified
until an objective kernel/owner verification covers current acceptance criteria.

Implemented boundaries: exact delegated tool/contract/resource/effect/reference
checks precede credential inspection and repeat at irreversible dispatch. Current
parent authority is checked at model/budget, task writes and final result commits.
An empty allowlist denies all tools. Root/mandate/day budgets remain shared;
delegation scopes add restrictions rather than refill the root allowance.
Reviewed-capability metadata binding also repeats the exact child permission
check before credential inspection; a catalog adapter cannot widen the original
resource, effect or credential-reference subset during binding. Installed
converters are appended before the final delegated catalog filter.

Catalog owns initial delegated source grants in the child-admission transaction.
Only selected source roots, immutable contract DATA and the child's own separate
session/task data may become child context bodies. Original transitive parents
remain pinned for privacy and current access checks; they cannot be selected as
additional model context merely because they occur in that dependency graph.
Unselected owner profile/user sources remain outside the child allowlist. Every
inherited project/scope grant is rechecked against the current parent permission
chain, including original purpose and access revision. Copying a child grant
does not preserve revoked parent access. Lifecycle/dispatch checks use durable
task/mandate state and need no surviving parent worker lease.

Child admission and owner controls take a task-tree PostgreSQL advisory barrier
before any ownership/session locks. Controls capture all independent sessions,
lock ownership rows in sorted order, then session admission and approval/run/task
rows. The barrier serializes only admission/controls for one tree; models and
unrelated task trees do not take it. A complete stop therefore cannot miss a child
admitted concurrently. Committed external effects remain journal evidence.

The owned context reader derives contract and selected roots from the current
run/task under its independent live lease, before ambient memory retrieval.
It pins the explicit current task before release, loads only selected root
bodies (64KiB in total), and retains canonical originals as privacy references.
Parent transcript, profile and retrieval bundles are never copied into a child.
Only the child's own transcript/task working state is added. Candidate contract
text remains Document/SubagentOutput DATA; a fixed kernel handoff label is the
input, not model-authored owner instruction. Checkpoints/approval continuations
retain selected references and reload current Catalog bodies after a new lease.
Saved references cannot select additional bodies. Current deletion, parent grant,
task revision and deadline checks repeat before model/dispatch/final publication.
The proposal source check also enforces the parent's exact scope/purpose grants
when the authenticated owner can independently read a broader Catalog body.

Pending acceptance: actual native durable separate-context scheduling through Broker,
dependency/width/depth bounds, independent and conflicting result collection,
parent crash and revision/stop races, resource/workspace arbitration, and an actual
single-versus-bounded-parallel experiment using the same shared model worker.
No criterion is satisfied by agreeing model outputs or a simulated effect.

## Owning manager component

`task.DelegationManager` admits an independent child/session, exact contract,
source grants, shared budget charge and bounded input on one PG transaction.
Owning adapters are mandatory; bodies stay in Catalog, handoffs use references.
Immutable proposal source/version and parent revision make replay repeat-safe.
One executor can explicitly expand to two; depth/tree/dependency caps stay finite.

Waiting children advance after a parent process disappears through trusted kernel
maintenance with current task/source/owner authority, never its ended lease.
Execution requires the child's own run/fence. Dependency edges stay within
sibling branches and refuse cycles; rejected dependencies block their dependents.

Actual completed-run results bind to strict Catalog reports containing action/
evidence/budget references, uncertainty and reason enums. Acceptance requires an
independent verifier adapter and never completes the task. Decision metadata has
only pinned references, not narrative or model authority. Conflicting reports
remain distinct. The [Task README](../../apps/orchestrator/internal/task/README.md)
documents constructor/adapters and lock order. Actual PG/source/budget tests use
two pooled connections; no model inference/effect is claimed. The optional
private Broker route and ordinary durable workers are composed as described
below; the shared native model experiment remains pending. Explicit
selected-source context is integrated in the ordinary preparation/recovery path;
actual Catalog and core projection/recovery checks are covered separately.

Trusted collection can generate a strict report on the same transaction from
the actual immutable final, current journal and conservative branch budget
reservations. No caller supplies accomplishments, action/evidence lists or cost
counters in that path. Its fresh Catalog DATA source is anchored in the child's
final-message graph and retains all originals. Repeated collection reuses the
same source; deletion or current authority failure rolls back publication.
Unknown effects and incomplete answers retain uncertainty, and generating a
report never verifies or completes either task.

An immutable historical final cannot free executor width while another attempt
for the same child revision remains nonterminal. Collection checks that boundary
under the existing task/session tree locks. Maintenance selects the newest final
for each eligible child and leaves a live attempt occupying its slot. Final
limitations remain conservative: independently unavailable effect evidence is
still missing after a separate task verification; unknown limitation kinds or
unresolved-action references that do not match that final deny collection.

The optional owned-context lifecycle adapter marks execution using the actual
child fence before releasing prepared input. Bounded kernel maintenance collects
committed finals first, then advances previously admitted waiting children using
the same existing durable input workers. Scan cursors affect fairness only;
they never replace PostgreSQL lifecycle or authority. Core composes these
adapters with the private Broker admission boundary only on operator opt-in.

## Operator composition

`BUTLER_DELEGATION_ENABLED` defaults false in core and Broker. Core composes the
real accepted-capability manager, private `/internal/delegation` handler, owned
context transition and bounded maintenance over its existing input workers.
`BUTLER_DELEGATION_MAX_PARALLEL` defaults one and allows only one or two; these
topology settings are operator environment values, never database overrides.
Broker's `BUTLER_DELEGATION_CONTROL_URL` must name the exact private route and
uses the existing action-authority token file. The runtime receives no token.
Fixed immutable `task.delegate` registration occurs only on explicit opt-in.
Core refuses activation unless migrations057/061 and their immutable contract/
snapshot triggers are available. Deploy migrations first, then matching core
and Broker flags/private route. Disabling the flag prevents new admissions and
maintenance; already accepted child inputs retain their ordinary durable
execution and current authority checks. Use the owner's task/tree stop or
mandate revocation to stop those branches.

The authenticated current action context advertises `delegation_ready` only
after core checks schema readiness and composes the manager/lifecycle adapters.
Broker hides the tool
without a current task/readiness and denies preparation before persisting an
intent. This bit supplies no authority: current lease, task/goal/mandate, source,
exact approval, dispatch, budget and accepted snapshot checks remain required.
Maintenance errors in one branch do not suppress unrelated initiative review.
The initial tool admits context-only research with an empty child tool allowlist;
the separate reviewed converter developer contract remains a distinct boundary.

## Current capability and result guards

`app.DelegationGuards` composes actual Catalog grant/source validation with a
mandatory local `DelegationPermissionReader`. Both adapters join the manager's
transaction; a network request or second pool connection under those locks is
forbidden. Permissions are exact tool, contract hash, resource, effect and
credential-reference subsets, with finite deadline and all eight budget bounds.
Current goal/mandate and prepare-only policy restrict that subset independently.
A registry entry, model tool description or old snapshot alone grants nothing.

`ReadDelegationCapabilitiesTx(ctx,tx,parent,childTaskID)` has an explicit phase:
empty child identity requires a current actual claimed delegate-dispatch proof;
nonempty identity resolves that child's accepted durable capabilities and current
parent/goal/package/source policy. Lifecycle must not depend on a surviving parent
worker. The private admission adapter persists its immutable migration061
snapshot during source binding, before the manager's first child lifecycle
check. The guard component alone supplies neither accepted capabilities nor
activation; operator composition selects the concrete admission adapter.

Report DATA is bound to a real completed child run, immutable final, exact current
task-result row, final artifact/message hash and original Catalog source graph.
Actual action intents, permanent effects, evidence and branch reservations are
read independently on the same transaction. The report cannot omit uncertainty,
invent actions or hide shared reservation usage. Accept requires independently
persisted owner verification for this exact result, criteria and version/hash
evidence; agreeing models or a model final do not verify it or complete a task.

Execution/source lifecycle guards remain active-only. Completed partial assistant
finals can be collected with limitations while the task is active. Failed,
timed-out and blocked attempts retain observations but require a separate
read-only historical collection contract before they can be collected: task
revision changes and absent immutable finals must not be relabelled as current
completion. Later journal reconciliation also makes an old final/report mismatch
fail closed; it needs a new explicit observation workflow. Stop/cancelled tasks
deny new collection even when historical result metadata remains visible.

See [guard acceptance evidence](../ai/delegation-guard-acceptance.md) for actual
two-connection PostgreSQL/race tests and the pending production/model criteria.

## Private dispatch admission component

The optional fixed `task.delegate` revision1 contract is a Broker-owned control
route with target `broker://task-delegation-v1`. Its effect is `prepare`; a
`local_runtime_ack` confirms only one committed child admission. Child research,
verification and parent goal completion remain separate. The immutable startup
contract must be registered through `RegisterOwned`; a registry entry cannot
replace its schema, target or effect. Runtimes receive no orchestration authority.

Model arguments contain an exact current contract DATA reference, selected source
references, all eight finite budget dimensions, deadline and bounded dependency
IDs. Unknown fields, duplicate keys at any depth, trailing values, null/missing
arrays and incomplete budgets are rejected. There is no owner, parent, lease,
permission, runtime URL, source body or credential field. Initial children have
an empty permission set; the parent's catalogue grants them no tools.

`broker.DelegationExecutor` runs only after normal Broker authorization and the
irreversible grant/effect claim. It sends the actual run/call/permanent-effect
identity, exact argument JSON and fixed contract hash to a private authenticated
HTTP `AdmissionService`, following the existing private authority bridge.
The operator composes `app.NewDelegationAdmission` with actual Task, Session,
Input and Catalog stores. Neither the constructor nor these files register a
public endpoint or enable a model tool. Tokens never enter model arguments or
runtime traffic; the HTTP client refuses redirects and bounds bodies/deadlines.

Service authentication alone is insufficient. Orchestrator derives its private
parent worker token from the actual inflight authorization, intent and executing
dispatch journal. In the manager transaction it rechecks canonical action/args/
effect fingerprints, expected fixed contract, current run/owner/autonomy/task/
constraints, lease epoch and current source/owner policy. Database time is reread
after acquiring dispatch locks, and the final proposal check repeats after budget
and queue waits. Existing-child `Admit` retries also repeat the proposal check
after lifecycle waits and before commit; routine maintenance does not acquire an
old dispatch dependency. The claimed-effect row is stabilized before its grant
and dispatch rows, matching outcome recording's order. The manager retains its
ownership/session/task/tree lock order;
all source/authority reads use that transaction, with no HTTP call or second
pooled connection under those locks.

Migration061 records one immutable metadata-only accepted capability snapshot per
child and per permanent effect. Source binding inserts it before the manager's
first lifecycle check, input publication and receipt. A queue/publication failure
rolls back child, session, grants, snapshot, input and child reservation together.
Its downgrade refuses a nonempty table. Content-free IDs, source versions/hashes,
exact binding and finite allocation preserve the admission evidence; source prose
remains deletable Catalog DATA.

Lifecycle uses the exact child identity to validate its accepted snapshot and
current parent/source/goal/mandate policy. It does not consult the ended parent
grant, approval or worker lease. The admitted deadline stays bounded by the
requested allocation and current owner policy, independently of transient parent
lease/approval expiry. Parent stop/revision, source correction/deletion/grant
revocation and current authority expiry still deny future work. Lost transport
acknowledgement remains uncertain; a spent effect is not automatically replayed.

The [admission acceptance audit](../ai/delegation-admission-acceptance.md) proves
the optional composition with an actual separate Broker process and pool2 PG.
Deployment wiring, real model scheduling and the shared-model single/parallel
quality/resource comparison remain pending. Failed/blocked report collection
continues requiring the separate historical observation contract described above.
