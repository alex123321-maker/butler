# Typed task experience inside Episodic Memory

`task-experience-v1` makes bounded, reviewable notes from the owning
Orchestrator's actual task/final/action/effect/usage observation. It introduces
no memory class, model, executable recipe or action authority.

The separate owner `POST /api/v4/memory/experience-observations` accepts only an
existing `run_id` with the normal owner authentication and delivery ID. The same
production adapter derives all current evidence and returns a content-free
Catalog source reference. It creates no model policy, job, attempt or spending,
changes no input/task/run/effect, and does not require a background model worker.
It preserves an explicit immutable witness before a separately authorized owner
control. An unchanged repeat reuses its seal; owner controls retain the ordinary
source staleness behavior. Caller-supplied bodies, verdicts, revisions, owners or
parents cannot enter this operation.

The authenticated owner selects an existing run and independent finite model
policy through `POST /api/v4/memory/model-experience-policies`. The owning
adapter resolves task identity, current constraints, immutable final result,
actual journal actions, exact historical tool contracts, observed sanitized tool
results and nullable durable input/output usage. It distinguishes the agent's
report, an explicit owner's task verification and effect evidence. The complete
original Catalog graph is a privacy dependency. Missing, changed or oversized
evidence denies creation instead of dropping a failure. The only supported task
revision advance is the existing verified completion transition bound to the
same exact owner's result; changed tasks require new evidence.

The owning Transcript API captures the exact bounded set of copied tool row
IDs, original run IDs and SHA256 fingerprints of stored tool name, arguments,
status, runtime, timestamps, result and error. Session/source revision is kept
as historical audit metadata. Seal and current Catalog guards lock/check the
captured rows; UPDATE or DELETE invalidates the observation, every descendant,
in-flight generation and an already released run pin. The thin model supervisor
uses the same metadata predicate. Append-only unrelated calls/messages and a
global privacy watermark do not change a captured row and do not revoke an
unrelated lesson. Message content remains protected by its full Catalog graph.
Migration 077 backfills a Transcript-owned metadata head for every existing
tool row and atomically updates its digest/revision/alive state on each writer.
Deletion retains a content-free tombstone, so identical same-ID reinsertion
cannot revive an old pin. Current supervision reads only these metadata heads,
not result/argument bytes. Full checks lock primary tool rows before inspecting
their heads, matching writer order. No raw result is returned by the fingerprint
or currentness SQL functions.

The existing durable model worker performs admission, quote reservation,
supervision, cancellation, usage observation and atomic publication. The model
can write a short summary, applicability commentary and at most twelve notes
pointing to actual action IDs. Code copies every typed action, failure, unknown,
version and verification status into the result. Model success wording cannot
change those fields. A single immutable `model_candidate` source and a neutral
`episodic` claim commit with the job checkpoint. Only existing owner `Decide`
creates the accepted `memory_episodes` projection. Repeated attempts retain
their immutable cases and contradictory observations; no vote or recency rule
turns repeated text into success. A later explicit owner verification creates a
new immutable observation/case for that original result; exact evidence retries
reuse the prior seal. Neither duplicate seals nor a later verification of the
same task/run count as another independent promotion case.

`BUTLER_MEMORY_EXPERIENCE_ENABLED` is operator-only and defaults off. The
code-owned `ReleaseQualified` gate is currently false. It cannot be enabled by
flags or stored settings. A later measured qualification is limited to the
measured provider/output/time/budget profile and never bypasses the separate
enabled worker/control/owner policy. No native quality result is claimed yet.

Current Broker dependency identity preserves status: both `active` core contracts and `enabled`
installed-extension contracts are supported by `actionauth.ContractHash`.
Disabled/unknown contracts remain unavailable, and changing status changes the
hash. No consumer normalizes status or replaces current task/tool permissions.

Runtime reuse also needs an explicit 64-character lowercase SHA256 operator
environment attestation in `BUTLER_MEMORY_EXPERIENCE_ENVIRONMENT_HASH`. Operators
must change it for runtime/API/configuration changes affecting applicability;
it does not automatically discover unreported external changes. The current
Broker-permitted contracts are fetched outside SQL. Exact environment and tool
hash/revision equality is required; missing/unknown versions suppress a lesson
and require fresh evidence. Mutable installed extension/converter revisions use
the existing reviewed-skills transaction validator. Current source scope,
ancestors, grants, accepted claim, deletion anchor and task/lease/input proof
remain mandatory. Raw owner inspection remains historical DATA.

The concrete read Broker path and ordinary episodic adapter filter incompatible,
unaccepted or negatively reviewed lessons. Typed input snapshots are excluded
from runtime recipe reads. Every released lesson adds a bounded immutable run
pin (at most eight). Subsequent provider/tool/final release checks current
environment, feedback, installed dependency versions and the complete Catalog
graph. Turning the feature off cannot resume a previously pinned lesson.

Applicability follows every lesson ancestor of a presentation, consolidation or
reviewed skill, rather than the returned root's type. A shared migration-077
metadata predicate checks the complete bounded ancestor set for current accepted
claim, environment and negative feedback. The owning private action context
supplies the operator environment fingerprint to existing reviewed-skill reads;
missing/disabled remains incompatible. Full Catalog proof precedes binding the
lesson pins, and normal run source graphs complete missing ancestry pins before
release. Eight unique ordinal slots enforce the union bound under concurrent
reads. Exact Broker contracts and mutable installed revisions keep their
existing owning validators; no Broker RPC is made under SQL locks. Historical
owner inspection remains DATA and does not confer runtime applicability.

Trusted evaluation composition may additionally restrict Reader exposure to an
exact frozen source subset before SQL ranking/body loading. This unexported app
option is not a Broker/model argument, grants no access and is ANDed with full
Catalog and experience checks. nil preserves ordinary production reads; an
explicit empty subset denies all. Parent grants needed for provenance proof do
not therefore expose source-task transcripts as the B/C notes treatment.

`POST /api/v4/memory/model-experience-feedback` accepts an owner-only exact
`source_id`, actual owner `run_id` and one explicit enum: `useful`, `misapplied`,
`correction_required`, `negative`, `unknown`. A delivery/run identity deduplicates
retries; changing the same observation is stale. Negative observations suppress
runtime reuse and are never overwritten by later silence or a useful vote.
Unknown remains unknown. Corrections create new current evidence rather than
rewriting historical feedback.

Skill manifests may declare exact `lesson_sources`. The existing private owning
verifier inspects full authoritative provenance even if this field is omitted,
so experience lineage cannot downgrade to the legacy single-trial path. An
experience promotion requires at least two distinct task/run originals, their
independent owner-verified results, confirmed matching actions/contracts, no
known failed/unknown step and current accepted compatible lessons. Result and
lesson parents become dependencies of the existing reviewed manifest. Existing
candidate/test/review/generation-CAS activation, correction, revoke and rollback
remain unchanged. Nothing autoactivates a skill.

Migration 077 adds only IDs, hashes, numeric-version dependencies and feedback
enums. All task/observation/lesson/recipe text stays in the existing deletion-aware
Catalog and Episodic projections. Privacy deletion and current independent
anchor reconciliation physically erase those copies. Immutable attempts,
reservation quantities, hashes and negative/unknown feedback metadata remain.
Down migration refuses populated history. Apply 077 before the new binary;
disable the feature and worker/control before rollback, retain history and the
current independent deletion anchor.

Controlled PG/Broker/lifecycle/actual dump-restore evidence and the separately
frozen A/B/C quality comparison are required before qualification. A fixture
pass, model report, successful transport or cheaper call count is insufficient.

Stopped originals use an explicit typed `terminal_attempt` when the owning
durable input is blocked/failed, has relinquished its claim/lease, and the actual
run is failed/timed out or stopped at a tool with an UNKNOWN effect. There is no
fabricated assistant final: result ID and agent report are empty, task verification
is unknown, and effects/failures are copied unchanged. Existing completed-final
observations retain their original schema/hash. Cancelled/paused tasks, active
inputs, waiting approvals and delegated originals cannot gain this permission.

The experience adapter alone calls the Go-only stopped-snapshot policy
constructor. Generic policy request JSON has no opt-in field and continues to
deny blocked tasks. That separately authenticated owner permission binds the exact
sealed observation/hash, one note turn/job, current task/constraints and current
goal/mandate/budget/deadline. Every schedule/claim/HTTP/stream/publication check
revalidates this proof. A note never unblocks/requeues the historical original.

Apply migration080 after077/079. Content-free input heads retain exact material
checkpoint hashes/revisions and deletion tombstones; supervision never loads the
large checkpoint. A shared per-input SQL fence orders source release with input
and run writers without taking primary input row locks under source/tool locks.
Populated terminal provenance prevents down migration. Candidate acceptance,
source privacy/restore, current applicability and the production qualification
gate remain independent.

Migration082 keeps subsequent owner deletion possible after copied tool rows
become stale. Its publication fence permits only the existing authorized exact
content-free working erasure, unchanged task identity and one revision advance.
It rejects private payloads, unknown/foreign deletion proof and material task
changes, while ordinary stale publication stays denied. Populated experience
provenance refuses downgrade even before a SQL deletion exists, because its
external anchor may already need replay. This exception grants no read, action
or learning authority.
