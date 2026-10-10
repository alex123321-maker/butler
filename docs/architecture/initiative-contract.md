# Initiative admission contract

[Lab acceptance evidence](initiative-acceptance.md) distinguishes actual
Broker/executor checks from recorded-journal adversarial fixtures and remaining
production source-hook/generative-planning verification.

An initiative starts with a typed observation and an active owner objective.
Observation content is data. Source configuration belongs to an authenticated
owner; only Goal Service can supply a current purpose/goal/mandate assignment.
Sources pin the ordinary task template and acceptance criteria. They also pin a
source revision, preventing an old observation from adopting widened config.

PostgreSQL stores delivery identities and normalized semantic fingerprints
across restart. The semantic key includes owner, goal, mandate, project, purpose,
condition kind, condition identity and resource revision. It excludes delivery
IDs and timestamps. Replayed evidence cannot increment distinct-failure counts.
Own changes, healthy states, empty checks, low confidence and low importance
produce bounded audit records or proposals, not model requests.

Promotion repeats source/current-condition checks under locks, then checks
current authority and a prospective first-turn budget on the same transaction.
No reservation or new shared budget pool is created by this preflight. Task
Service creates the ordinary task and Input Service creates its session-ordered
receipt inside that transaction. Actual inference/effects/children retain normal
durable budget admission, exact-action grants, result verification and owner
stop/pause behavior. A final current-time authority check follows budget waits.

Only a verified Task Service completion with a completion result can complete
the initiative. A provider final answer or completed run is insufficient.
Already-admitted resource/condition identity cannot be reopened by new delivery
metadata. Healthy/new resource state makes an unadmitted old candidate obsolete.

Tick state uses leased/fenced acknowledgement and deterministic event IDs.
Calendar schedules use embedded IANA rules: missing minutes skip, repeated
minutes execute their first occurrence only. Interval schedules retain their
phase. Downtime creates at most one latest pending check per schedule; it never
creates one task per missed interval. Owner changes invalidate old tick/source
revisions before their observations are recorded. A late claim cannot acknowledge
a new worker's lease.

Quiet checks/proposals are exposed through an owner-authenticated pull view;
source active hours, confidence/importance, cooldown and an immutable UTC-day
attention cap of 20 admitted tasks limit interruption. Expected model calls and
attention are candidate estimates; real model and external-effect charges remain
separate execution-budget dimensions.

The installed `task_project` observer reads only current owner-bound Task/Run
state in PostgreSQL. Owner task admission supplies an optional exact project
reference and SHA-256 resource revision; source configuration pins the same pair.
Distinct failed/timed-out attempt identities count once. Raw run error text is
never selected. Newer verified project results clear the condition; provider
final answers cannot. Initiative-origin attempts are suppressed. Promotion checks
current attempt/revision and absence of a newer verified result again after
budget waits. An observation is still data and cannot grant a tool permission.

An owner may opt into `digest_channel` matching the source's stored Web/Telegram
session. The separate migration056 outbox groups admitted/verified-result changes
into count-only immutable notices: one batch per six-hour UTC window and at most
four batches per owner UTC day. Empty checks, proposals and duplicate observations
produce no notice. Configuration, goal/mandate and active hours are rechecked
before delivery. Changed authority fails the pending notice; inactive hours defer
it. The channel worker shares the existing bounded absent/ACK/unknown semantics,
including no automatic repeat after an uncertain send. These notices are not model
final results, effect evidence or claims of user read. Owner view exposes their
delivery states separately. All template/project/error contents are excluded.

Current scope: authenticated normalized event ingress, durable schedules, the
installed PostgreSQL project-attempt observer, grouped digest delivery and atomic
ordinary-task admission. An unconfigured heartbeat remains an empty check. Fixed
external source adapters and the repeated-error to verified prepared-project-fix
acceptance remain outstanding. This contract
does not claim issue #15 acceptance from foundational tests alone.

## Recorded project checks

`broker_project` pins a reviewed immutable extension manifest, one GET/read
operation, exact HTTP(S) resource and the owner's exact project revision. The
configured URL only selects an already recorded approved Broker observation;
it launches no network request and grants no egress permission. Normal Broker
public-address, resource, credential and approval checks remain authoritative. Its
PostgreSQL adapter sends no external request and spends no model turns. It reads
only saved completed Tool Broker calls with the matching historical owner review,
completed authorization and dispatch, and confirmed read evidence. It checks
the exact intent/action/argument hashes and current task/constraint/project
revisions. Responses must carry bounded unique-key `project_check_v1` JSON.
Only condition/evidence hashes and identifiers leave the parser. Raw error text,
package instructions, unknown JSON members and `produced_by_butler` in a response
cannot grant authority or set origin. Origin comes from Task Service.

One run counts as one attempt, even if the provider asks for the same check
several times. A stable check identity/error class on the same source revision
coalesces three distinct attempts. A passing newer check, different latest
condition, or a newer verified prepared-correction result invalidates an older
candidate before promotion. The exact stored fact is checked again after budget
waits. Source lifecycle deployment must wire `catalog.CheckRunSources` into
the observer and `catalog.CheckRunSourcesTx` into its store's `RunDataGuardTx`.
The recorded-check adapter and promotion fail closed without these hooks; this
staged branch needs the source-lifecycle bootstrap integration before
production acceptance. Immutable manifest lookup during promotion uses the same
owned transaction and cannot borrow an additional connection while locks are held.

The owner UI exposes inspection, pause/enable controls, active hours, templates,
source revisions, daily/interval schedules, deterministic decision reasons and
separate grouped notice delivery states. Connector checks are selected from
reviewed read operations; source setup pins the selected manifest rather than
acquiring any new capability. The token stays in page memory. Changing proof
clears the view and stale asynchronous responses cannot restore it. New sources
start paused. This UI does not mark tasks completed or authorize an action.
