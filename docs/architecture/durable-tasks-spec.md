# Durable task implementation

Implemented for issue #4, 2026-10-11. This document describes current behavior;
the broader product contract remains in `persistent-assistant-spec.md`.

A session owns identity, transcript and execution ordering. A task owns a goal,
source, owner, acceptance criteria, constraints and a working area. A run is one
attempt. Conversation runs may have no task. No model final event, run completion
or artifact can automatically complete a task.

## Ownership and persistence

`apps/orchestrator/internal/task` owns PostgreSQL `tasks` and `task_results`.
Task IDs are independent random IDs. `runs.task_id` is optional; `task_revision`
pins the version admitted before execution. A parent task reference associates
subtasks without introducing a scheduler or granting additional authority.

`tasks.working_json` stores the plan, pending actions and scratch state. Its
`working_revision` uses compare-and-swap updates. Goals, constraints and
acceptance criteria are separate durable task fields; constraints have an
explicit revision. Task state transitions increment the task revision. Criteria
and constraints are immutable in this first API; amendments and effect-bound
revocation are subsequent work in issues #7/#9.

Task context is loaded only through an explicit `payload.task_id` on ingress.
The active task must belong to that session and owner. There is no inference
that a new message continues the previous failed task. A normal question has no
task reference and creates no task row. Task working state never inherits
session-scoped `memory_working`. Transcript remains session-owned and available
across task boundaries as history, not as the active task goal.

Orchestration checkpoints update only the task attached to the run and only
while its pinned revision is current and active. A model final stores an
`unverified` task result, leaving the task and unfinished work intact. A failed
attempt blocks its task without deleting work. Transient Redis session/run
scratch may expire or be cleared; it is never the durable working area.

## Task lifecycle and verification

Statuses are `active`, `waiting_for_user`, `blocked`, `paused`, `cancelled` and
`completed`. Owner control operations can move between nonterminal statuses.
`cancelled` and `completed` cannot be reopened. Continuing a waiting, blocked or
paused task requires an explicit transition to `active` and an explicit task
reference on the next run. Awaiting tool approval remains a separate run state.

Completion requires a saved verified result for the current task revision,
nonempty evidence references, a verdict for every acceptance criterion and
explicit owner confirmation. A model response is recorded separately as
unverified even when its run is `completed`. Verification records persist the
summary, run reference, evidence references and checked criteria. References
are evidence identifiers; the kernel does not independently evaluate external
artifacts in this implementation. Automated effect verification belongs to
issues #11–#13.

The owner control API follows the deployment's existing trusted API boundary.
It is absent from the model tool catalog. `owner_confirmed` records an explicit
control request; it is not a new authentication mechanism. API authentication,
runtime egress restrictions and credential policy remain separate work in
issue #31. Current cancellation prevents new task admission/checkpoints; stopping
in-flight effects and cancellation of a child tree belong to issue #9.

## API and transition compatibility

The session must already exist through the Session Service or a conversation
run. Owner-controlled operations are:

| Endpoint | Behavior |
| --- | --- |
| `POST /api/v3/tasks` | Create with `session_key`, `owner_id`, `goal`, `source`, `constraints`, nonempty `completion_criteria`, optional `parent_task_id` |
| `GET /api/v3/tasks?session_key=...` | Read durable task records, latest 50 |
| `GET /api/v3/tasks/{id}` | Read working state, attempt IDs and saved results |
| `POST /api/v3/tasks/{id}/working` | Save `working` with `expected_working_revision` |
| `POST /api/v3/tasks/{id}/status` | Transition using `expected_revision`, `status`, and a verified `result_id` for completion |
| `POST /api/v3/tasks/{id}/results` | Save current `task_revision`, `summary`, `evidence_refs`, all `verified_criteria`, optional `run_id`, and `owner_confirmed: true` |
| `POST /api/v1/events` | Continue the active task with `payload.task_id`; omit it for a conversation |

Invalid evidence/criteria produce 400; stale versions and inadmissible task
states produce 409. No task control action is delegated to model transport.

`/api/v2/tasks` and compatibility `/api/tasks` now read durable tasks and attach
the latest attempt's diagnostics. They expose `task_status`, task revision,
`legacy` and `result_verified` separately from `run_state`. Detail adds
`durable_task`, all `run_ids` and results while retaining the prior summary,
source, waiting, error and artifact fields. Artifact/activity routes aggregate
the task's attempts; debug uses the latest attempt. `/api/v1/runs` and existing
run URLs remain available for conversations and individual attempts.

Migration `029_durable_tasks` retains every old run/transcript and its URL. Old
run IDs become explicitly marked legacy task IDs, not retroactively verified
work. Completed/failed/timed-out legacy runs are `blocked`; old final answers
are unverified results. Matching session working snapshots are copied only to
their recorded run's legacy task; legacy JSON plans/entities remain in scratch
without losing arbitrary historic shapes. Original `memory_working` remains
available for audit. Down migration removes the new task schema and links,
preserving original runs, messages and legacy working snapshots; new task-only
data must be backed up before an operational rollback.

## Acceptance evidence

Lab PostgreSQL tests cover one task with four prepare/correct/approve/send
attempts, owner waiting and explicit continuation, preservation after a new
service instance, successful-run retention, failed A/new B goal isolation,
ordinary conversation without task rows, CAS conflicts, child links, pause,
block, cancel, and completion only with all criteria/evidence. A separate
disposable migration database verifies upgrade/downgrade and legacy results.
`BUTLER_TASK_MIGRATION_POSTGRES_URL` must point to that exclusive database and
differ from `BUTLER_TEST_POSTGRES_URL`.

API tests and a functional browser fixture show a waiting task whose latest
attempt is completed, two attempts, saved plan/constraints, and an unverified
model response. These tests use deterministic transport fixtures; they do not
claim that an external message was delivered or an arbitrary model followed a
procedure correctly. Durable ingress/restart of an executing run (#5), durable
approvals (#6), exact action authorization (#7), constraint amendments/tree
cancellation (#9) and outbox delivery (#12) remain separate issues.


Versioned owner amendments, inherited restrictions and cancellation are implemented by [Owner Controls](task-owner-controls-spec.md).

## Verified task notification mode

The Task-owned immutable `result_delivery` is `attempt` by default. Authenticated
initiative Source configuration may select `verified_task` for a newly admitted
root; child admission inherits it and cannot override it. Its external delivery
is governed by the owner completion/result transaction described in
[atomic result delivery](atomic-result-delivery.md). Completion and delivery remain
separate states. One frozen root notice requires current owner verification of
original criteria and an unredacted complete actual attempt. Child technical
return, successful effect or unverified model final never satisfies this gate.
Neither owner replay nor delivery reconstruction creates another root notice.
