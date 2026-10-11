# Durable bounded delegation

Implementation in progress for issue16. This foundation is not a production
delegation release. No model-selectable executor tool, child scheduler or owner
delegation endpoint has yet been enabled.

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

Pending acceptance: actual durable separate-context scheduling through Broker,
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
two pooled connections; no model inference/effect is claimed. Production explicit
Broker/scheduler and the shared model experiment above remain pending. Explicit
selected-source context is integrated in the ordinary preparation/recovery path;
actual Catalog and core projection/recovery checks are covered separately.

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
worker. The production adapter must persist its accepted snapshot during source
binding, before the manager's first child lifecycle check. No production reader
or accepted-snapshot storage is enabled by the guard component itself.

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
