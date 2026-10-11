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

Pending acceptance: actual durable separate-context scheduling through Broker,
explicit source-only context without parent transcript/profile inheritance,
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
source context without ambient profile/retrieval, Broker/scheduler and the shared
model experiment above remain pending.
