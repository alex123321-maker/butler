# Agent-created scheduled jobs

Implemented as optional fixed kernel capabilities. PostgreSQL owns accepted jobs,
control receipts, occurrences and reminder notifications (migration085). No new
service, dependency, provider session, Redis queue or execution runtime is added.
Production activation is an operator deployment decision, separate from tests.

## Ownership and agent integration

`internal/scheduling` contains the shared clock calculation used by initiative
checks and scheduled jobs. Initiatives retain their owner-configured sources,
observation leases, thresholds, cooldown and attention policy. A scheduled job is
an explicitly authorized future instruction; it does not fabricate an observed
failure or pass through project-failure thresholds.

`apps/orchestrator/internal/scheduled` owns job records and transactional
occurrence admission. Existing Input maintenance advances a bounded batch each
second. Because admission is one PostgreSQL transaction, no independent worker
lease/queue is needed for this local step. Session row locking and deterministic
`(schedule_id, revision, slot_key)` uniqueness serialize concurrent maintenance.
Task and Input own all execution after admission.

Tool Broker registers five immutable `RegisterOwned` contracts targeting
`broker://schedules-v1`. The normal `ListToolsForRun` path supplies them to model
transport only when current owning composition is ready. Every call receives
ordinary exact Prepare/Authorize, permanent dispatch, shared tool budgets and
trace. The private `/internal/schedules` HTTP endpoint repeats actual journal,
grant, lease/fence, task/constraints, source and accepted-input-claim checks.
Private service authentication alone cannot create a job. Responses and errors
contain no private token or reusable execution authority. The fixed operator
endpoint disables ambient proxies and redirects and bounds time and body sizes.

| Tool | Model arguments |
|---|---|
| `schedule.create` | `{job}` |
| `schedule.list` | `{limit?, after_id?}`, default5/max10, opaque cursor; brief metadata |
| `schedule.get` | `{schedule_id}`; latest20 occurrence states |
| `schedule.update` | `{schedule_id, expected_revision, job}`; complete replacement |
| `schedule.cancel` | `{schedule_id, expected_revision}`; permanent |

Owner, session, run, original task constraints/policy, approval and assignment are
derived from checked private state. The model cannot choose them. Viewing and
mutation are limited to the same owner and conversation. Delegated executors do
not receive these capabilities. Scheduled occurrences cannot mutate schedules;
a fresh owner conversation is required. Exact cancellation remains
subject to current task/goal/tool policy and never undoes external effects.

## Stored instruction and authority

`job` contains `title`, `mode`, `instruction`, `completion_criteria`, `timing`,
`expires_at`, `max_occurrences` and `paused`. A nonempty instruction is bounded
to8192 bytes (4096 for reminders), title256 and up to16 criteria of1024 bytes.
Agent tasks require at least one criterion. All objects reject unknown fields,
duplicate keys, trailing JSON, invalid/null required fields and excessive nesting.
Encoded arguments are capped at64KiB and results at128KiB. List omits full
instructions/criteria; get opens one job. Recognized raw credential material is
refused before persistence.

Creation requires an actual exact owner-approved action or a current assigned
owner-goal mandate permitting scheduling and initiative. An exactly approved
future job may use that existing assignment without enabling general initiative.
Without a broad assignment, exact approval authorizes only this finite schedule;
future tools retain ordinary autonomy/approval and credential policy. An approved
create never invents a goal, mandate, resource permission or owner Actor.

The original goal/mandate/purpose revisions remain pinned. Every occurrence and
later task/model/tool/result boundary rechecks current authority. A changed or
revoked mandate cannot silently rebind to its successor. A source task may finish
without revoking its independently accepted schedule; changed constraints/goal,
prepare-only policy, pause/wait/cancel or redaction invalidate it. Historical
source checks never borrow the original run's lease. Source deletion/restriction
withholds saved instruction text from list/get and receipt replay, and prevents
new execution/delivery. A different goal/mandate scope sees metadata only, so an
inspection tool cannot release a local-only job's text into another provider
scope. Immutable records retain audit identity.

Schedules have a maximum30-day horizon and1–100 occurrences, with at most100
active/paused/blocked jobs per owner. Expiry must fit an assigned mandate's goal,
expiry and original budget period. Update does not reset occurrence usage and
cannot extend expiry or the original maximum. Automatic updates/cancellation
under a mandate may affect only jobs with its compatible original assignment.
Shared root-task, mandate and owner-day budgets remain authoritative. Prospective
root/day admission creates no reservation; each actual inference/action is
charged normally. A budget or authority denial is inspectable as `blocked` and
requires an authorized update; it does not repeatedly call the provider.

## Time semantics

`timing` requires `kind`, explicit IANA `time_zone`, `missed_policy` and
`grace_seconds` (0–3600). Host `Local` and an empty timezone are rejected.

* `once`: an RFC3339 `at` **or** positive `after_seconds`, maximum30 days;
  `max_occurrences` must be1. The server resolves relative time once inside the
  idempotent creation/update transaction and returns the actual UTC `next_at`.
* `daily`: local `hour`/`minute` and optional unique `weekdays` (Sunday0).
  An empty day selection means every day. Missing DST minutes skip; a repeated
  local minute runs its first occurrence only.
* `interval`:60–604800 seconds. Tick advancement preserves phase. Pausing or
  resuming without changing timing preserves the saved due time.

Downtime coalesces to the latest due slot or skips an expired grace window.
At most one occurrence is admitted per maintenance transaction; historical slots
are not queued. Each consumed slot, including a skip, counts toward the finite
maximum. Changing configuration increments its revision and fences old work.
Fresh PostgreSQL time is checked after authority/budget waits; UTC-day rollover
restarts admission rather than using the preceding day's prospective allowance.

## Execution, overlap and results

`agent_task` atomically inserts an occurrence, a fresh ordinary root Task and an
ordered `SCHEDULED_INTERNAL_EVENT` Input receipt. It preserves constraints and
execution policy. It does not keep the creation run alive or restore a provider
session. The ordinary Session/Input/Core path chooses current transport/context,
checks model budget/privacy and sends every tool proposal through Broker.

One unfinished input per schedule prevents overlap. A completed/failed/expired/
cancelled attempt releases overlap; an unverified Task may remain active and
does not indefinitely prevent recurrence. Waiting/blocked inputs still hold it.
An unresolved external effect stops the schedule with `effect_unknown`.

`reminder` queues only immutable stored text in a reminder outbox, spending no
model call and creating no fake run/result. The existing delivery worker and
Telegram sender handle its chunks. The current owner/session, job revision,
source and authority are checked again before sending. Web exposes `result_ready`
through schedule inspection; it does not claim an active browser notification.
Explicit absence can retry within six attempts/24hours. Lost/expired sending
acknowledgement becomes `delivery_unknown` and never automatically resends.

Cancellation/update/pause fences queued and running occurrence publication,
including descendants, at normal task authority checks. Input Service atomically
retires nonterminal inputs of obsolete revisions so a blocked old occurrence
cannot starve its conversation after a resume. Previously confirmed effects/ACKs
and unknown-effect checkpoints remain history.
It cannot recall a message already accepted by Telegram. Task execution/result
verification and channel delivery remain distinct: a schedule may be `finished`
while its final admitted attempt is still executing. Occurrence inspection shows
input/run state, task status, result verdict and delivery separately. Successful
model output remains `unverified`; no new automatic completion verifier is added.

## Operator configuration and acceptance

Both services default `BUTLER_SCHEDULES_ENABLED=false`. Enabling requires the
existing private action authentication and migrated PostgreSQL. Tool Broker also
requires fixed `BUTLER_SCHEDULES_CONTROL_URL`. Orchestrator refuses partial schema
or disabled immutable-history triggers before advertising readiness. Database
settings cannot enable this capability or choose its private endpoint.

Compose overlay: `deploy/docker-compose.schedules.yml`, combined with the normal
Compose file and `BUTLER_SCHEDULES_ENABLED=true`. Existing owner autonomy,
mandates, allowed tools and explicit approvals still apply. There is no deployment
or automatic activation in the implementation validation workflow.

Run `pwsh -NoProfile -File scripts/dev/schedules-local.ps1`. It creates an isolated
disposable pgvector PostgreSQL container, mounts this checkout with repo-local
Go caches, builds the actual Broker and runs mandatory tests under race detection
with no skips, then existing authority/budget/initiative compatibility and vet.
The mandatory report is `.cache/schedules/targeted.jsonl`. No native/paid model
call or production credential/deployment is used. The full-path test injects a
scripted provider and independently checks ordinary Input/Core final persistence,
unverified verdict and Web result delivery. This is component/integration evidence,
not a claim of live model quality or human Telegram delivery.
