# Owner goals and durable autonomy

This module owns versioned confirmed objectives, preferences and constraints,
and separate proposed assumptions. Only typed authenticated owner decisions can
create or amend goal and broad mandate authority. Agent assumptions cannot
promote themselves; explicit owner confirmation creates a distinct objective
with the assumption reference retained in its provenance.

`RevokeMandate` is the finite authenticated channel command for an exact saved
mandate/version. Its durable decision replay runs before CAS; it revokes current
authority and queues branch cancellation without changing budgets or prior effects.
The actual PostgreSQL owner-reference tests verify foreign-owner/stale denial,
immediate admission denial, propagation and restart/lost-ACK deduplication.

`CheckAdmissionTx` is for trusted initiative coordinators. Acquire ownership and
session locks first, call the typed check, and insert new task/input rows in the
same transaction. `task.CreateTx` inherits a parent's exact assignment and
refuses stale authority. Every task records goal/mandate revisions, purpose,
initiative rationale and observation reference. Ordinary tasks remain valid
without any goal assignment.

An explicit authenticated owner task may use an active scoped mandate with
initiative disabled. Only that private manual path skips the initiative flag;
`CheckAdmissionTx` always requires it for scheduler work and retains all other
current authority checks.

Checking lock order: ownership, session/run/task, mandate, goal, purpose-policy,
shared budget. Confirmed preferences/constraints fence all older assignments of
their exact owner/purpose and remain full protected text in fresh context.
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

Transient authority deadlines are separate from immutable budget periods.
Dispatch/model/child admission rechecks DB time after budget waits and clamps
runtime/model contexts to the earlier goal, purpose policy or active-hours end.

`CheckExtensionAttachmentTx` is the private same-transaction permission gate for
an installer with independently verified manifest bytes/source. It permits exact
reviewed capabilities, not arbitrary code execution or later runtime effects.
The `/goals` owner page and authenticated `/api/v3/owner-*` routes expose goals,
assumptions, bounded mandates, current obligations and pending propagation.
See [owner authority](../../../../docs/architecture/owner-goals-mandates-spec.md)
and [actual acceptance checks](../../../../docs/architecture/owner-goals-mandates-acceptance.md).
