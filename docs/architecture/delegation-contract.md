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
