# Initiative admission contract

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

Quiet checks/proposals are exposed through an owner-authenticated pull digest;
source active hours, confidence/importance, cooldown and an immutable UTC-day
attention cap of 20 admitted tasks limit interruption. Expected model calls and
attention are candidate estimates; real model and external-effect charges remain
separate execution-budget dimensions.

Current scope: authenticated normalized event ingress, durable schedules and
atomic ordinary-task admission. The default heartbeat is an empty check. Fixed
external source adapters, active channel digest delivery and the repeated-error
to verified prepared-project-fix acceptance are not yet complete. This contract
does not claim issue #15 acceptance from foundational tests alone.
