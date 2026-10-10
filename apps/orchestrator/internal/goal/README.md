# Owner goals and durable autonomy

This module owns versioned confirmed objectives, preferences and constraints,
and separate proposed assumptions. Only typed authenticated owner decisions can
create or amend goal and broad mandate authority. Agent assumptions cannot
promote themselves; explicit owner confirmation creates a distinct objective
with the assumption reference retained in its provenance.

`CheckAdmissionTx` is for trusted initiative coordinators. Acquire ownership and
session locks first, call the typed check, and insert new task/input rows in the
same transaction. `task.CreateTx` inherits a parent's exact assignment and
refuses stale authority. Every task records goal/mandate revisions, purpose,
initiative rationale and observation reference. Ordinary tasks remain valid
without any goal assignment.

Checking lock order: ownership, session/run/task, mandate, goal, shared budget.
Owner mutation locks mandate then goal only. It commits new status/revision and
a durable propagation job without waiting on worker ownership. Claim, model
admission and atomic result persistence must hold the current authority locks
until their transaction commits. Propagation jobs cancel or pause old branches;
pending means immediate authority denial is in force while branch cancellation
still awaits a retry. External effects and saved results are never rolled back
or described as undone by a revocation.

Broad mandate allowance is finite and shared across tasks. Its original budget
deadline and limits do not reset on revision, resume or owner edits. Existing
exact task mandates remain supported by Tool Broker's private operator API;
broad goal mandates are authored through the authenticated owner service only.
