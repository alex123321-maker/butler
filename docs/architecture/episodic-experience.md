# Verified observations and reusable episodic lessons (#90)

The implementation belongs to Episodic Memory. Transcript remains the original
record, Working Memory remains current task state and Profile Memory remains
accepted scoped facts/preferences. A typed lesson is immutable DATA, not hidden
reasoning, permission or task-completion proof.

See the [owning implementation contract](../../internal/memory/experience/README.md)
for bounded extraction, current environment/dependency applicability, feedback,
promotion and privacy behavior. The Orchestrator seals evidence through typed
task/result/transcript/action/usage APIs; the existing background worker owns
finite extraction attempts and atomic publication. Tool Broker retains all tool
validation, policy, credentials, approval and effect mediation. No lesson can
remove a required approval or grant a tool.

Success is three separate observations: an agent's report, an explicit owner
verification and actual independently confirmed effects. Known failures and
unknown effects are never transformed into confirmed success by generation,
coalescing, retrieval frequency or a helpfulness vote. One source case remains
one immutable lesson; bounded reads can return several cases without replacing
their contradictory observations with a fabricated consensus.

Read eligibility is additional to current Catalog access. The environment
fingerprint is an explicit operator attestation, not an autonomous claim that
all external systems are unchanged. Unknown or mismatched versions require
revalidation. Current tool contracts are obtained outside SQL, installed mutable
tool revisions are checked inside the release transaction through the reviewed
skills validator, and every used lesson has a current run pin. Final source,
task/constraint/lease/input and lesson guards remain independent.

Copied tool observations additionally pin the exact original tool row IDs/run
IDs and stored-data SHA256 fingerprints through the owning Transcript API.
Changing or deleting a captured row invalidates all Catalog descendants and
current read/publication pins. Historical session source revision is audit
metadata: appending unrelated calls/messages or replaying a global deletion
watermark cannot revoke unchanged unrelated experience. Original message data
continues to use the complete Catalog parent graph. This is separate from the
whole-session copied-summary coverage predicate in migration 072.

The same applicability requirement follows all lesson ancestors of any derived
presentation, summary, consolidation or reviewed skill. Current accepted claim,
environment and nonnegative feedback use one bounded metadata predicate; source
proof and existing exact tool/installed-dependency checks remain independent.
The private owning action context supplies the configured environment hash to
reviewed-skill reads. Runtime reads pin every relevant lesson ancestor, and
final guards complete pins from the full current Catalog run graph. Unknown
environment fails closed; an ordinary source without lesson lineage retains
its behavior. Up to eight lessons can be pinned, with a concurrent-safe slot
bound. No model header/configuration value can supply that environment.

Current dependency preparation uses the Broker's canonical contract validation:
`active` and `enabled` are both usable statuses, including installed extensions
which expose `enabled`. The exact status stays in the contract hash; neither
status conversion nor unknown/disabled tools can satisfy an earlier dependency.
Current permitted catalog, installed version, environment and full ancestry
checks remain independent of that validation.

Experience-derived skills use the existing #19 lifecycle. The private verifier
derives experience lineage from complete Catalog parents even when a caller
omits `lesson_sources`; repeated independent confirmed original tasks and exact
current accepted lessons are mandatory for that path. Owner review/test/CAS and
rollback are preserved. A lesson with failure/unknown evidence may teach what
to avoid, but cannot qualify as a verified successful procedure.

The feature and its qualification constant remain off pending controlled
security/lifecycle/restore gates and an independently frozen #78 A/B/C screen:
no episodic experience; own notes without skills; those same notes plus reviewed
skills. Original tasks generate their own notes from actual actions/results.
Human-authored recipes or control answers cannot populate the note corpus.
History, summaries and caches must not leak excluded experience into A or skill
instructions into B. Report B-A and C-B separately, actual task/effect outcomes,
mistake repetition, wrong application, owner interventions and complete creation,
write/search/use/skill preparation/test/load cost. Unknowns, no gain and
regressions remain valid outcomes; #76 behavioral error catalog stays separate.

Explicit startup enablement also requires the measured native profile: Gemini
Flash High, exact32768 output tokens, actual provider timeout at least300s and a
matching durable candidate budget. An enabled but unqualified or unsupported
configuration fails startup with typed evidence failure. A disabled feature
constructs no experience store. Read/context composition is independent of the
global model-jobs flag; background generation additionally needs that flag and
its actual handler. Doctor reports these separately as `experience_context`
and `experience`. Neither composition state authorizes a job or proves quality.
The ordinary source guard remains installed when the experience store is absent.

The owning memory.facts path applies the same current lesson-ancestor and exact
trusted exposure checks before reading accepted snapshots. Owner acceptance of
a derived episodic/profile value does not remove the lesson's environment, tool
contract or negative-feedback conditions. Legacy facts with no lesson ancestry
retain their ordinary authority checks.

An explicitly selected stopped original can also produce a lesson without an
assistant final. Its typed `terminal_attempt` records the relinquished durable
input identity/status, attempt count, exact input metadata head and actual run
state. `result_id` and `agent_report` stay empty; owner verification stays unknown.
The owning input service proves that no claim/lease is executing that original.
Cancellation, waiting approval, running input, arbitrary error text and delegated
tasks cannot use this path. An uncertain external effect remains UNKNOWN even
when a private recipient audit knows that bytes were persisted.

Only the experience adapter can construct the separate one-use model policy for
that exact sealed snapshot/hash. This Go capability is absent from the public
generic policy request; generic jobs still reject blocked tasks. Scheduling,
admission, supervision and publication recheck the current snapshot, task and
constraints, owner policy, root goal/mandate, deadline and finite budget. The
original input/task is never requeued, unblocked or completed by learning.

Before a distinct owner control, an authenticated owner can seal the exact current
observation with `POST /api/v4/memory/experience-observations` and only
`{"run_id":"existing owning run"}`. A delivery ID is required. The owning adapter
derives all evidence and returns only a content-free Catalog source reference;
client-supplied body, parents, task revisions, owner identity or verdict are
rejected. This operation creates no model policy, job, attempt or allowance and
does not change the original input, task, run or effect. Repeating an unchanged
observation reuses the seal. Current source/privacy/authority requirements still
apply, and an owner Resume makes the old terminal snapshot stale normally.

Migration080 maintains content-free input hash/revision/tombstone heads and
orders source release against material input/run changes with a per-input SQL
fence. No full checkpoint body is reread by supervision. Schema rollback refuses
populated stopped-attempt provenance, and normal source deletion/restore fences
continue to erase copied bodies while preserving unknown outcomes and spend.

A distinct `reconciled_attempt` may supplement that stopped original after a
separately authorized ordinary owner Resume. The owner first seals the original
through the observation endpoint, then explicitly admits a continuation on the
same task. The existing controller owns cancellation/history, revision changes
and queued continuation admission. Fresh normal grants and the unresolved-effect
guard restrict the continuation; this memory adapter supplies no execution
capability. Only one actual independently journaled `http.request` read of the
original recipient identity, accepted by the existing private Reconcile contract
as ABSENT, can supply this supplement. No original mutation is retried.

The owner note request may select `reconciliation.original_snapshot_id` and
`reconciliation.control_id` alongside the original RunID. The owning input API
checks the exact original input/checkpoint against the pre-control immutable
metadata witness, owner Resume transition, revision change, actual queued
continuation, its completed run/final and independently captured lookup evidence.
It never reads the stale observation body. Primary original and continuation
messages/used sources, plus the actual owner-control message, remain the complete
copy-bearing privacy parents. Witness metadata/access, current task authority,
original input links and exact control/final/effect/tool rows remain separate
release requirements. Deleting or withdrawing the selected witness denies reuse.
Its metadata reference is also an explicit privacy edge: deletion/revocation
recursively erases the reconciled observation, accepted note/episode and further
derived or cached copies. This edge never grants body access to the stale
witness. Independent primary parents remain separately owned and readable.

Original RunID/revisions, UNKNOWN runtime action, failures, missing final/report
and unknown owner verification stay unchanged. The supplement names the distinct
current revision, owner control, continuation input/run/final/report, authorized
lookup and current ABSENT evidence. Original and continuation usage are separate;
neither a successful read nor its final becomes original task success. The old
terminal snapshot stays stale; this creates a new immutable observation and one
fresh owner-authorized note policy under the unchanged finite source/day budget.

Migration081 adds content-free exact row heads/pins, including the selected
control link and continuation event omitted from the ordinary terminal projection.
The pre-Resume seal also records each original typed action's hash, observed
effect and evidence references. Reprojection requires the exact original tool
pins and reconstructs those historical actions from current owning rows against
that witness. It cannot substitute a corrected response or current reconciled
effect for an original outcome. A pre-081 witness must be sealed again while it
is still current, before Resume, to record these action witnesses.
The Catalog locks the complete source/witness set, then the union of original,
lookup and ancestor tool rows, input fences and metadata fences. The owning seal
prelocks that full union before checking a parent's access, so an original that
already read another lesson cannot acquire an ancestor input before a new tool.
Supervision/publication reads only those
small heads; it never reloads checkpoint/instruction/response bodies. Material
writers wait for a current release; identical delete/reinsert retains tombstones.
Generic policy JSON cannot construct this handler-only one-use proof and generic
blocked-task policy rules are unchanged. Apply081 after080; down migration refuses
populated reconciliation or action-witness history and restores080 predicates
only when empty.

Final publication checks only applicability pins already captured by that run.
The owning completion proves its exact owner/task/constraints operation and calls
Catalog `CheckRunSourcesTx` first for current source privacy and revisions. The separate
`CheckCapturedPinsTx` then requires an exact set of existing lesson pins and
checks accepted claims, feedback, environment and installed dependencies without
reading bodies, creating pins or repairing a missing pin. Generic `AccessTx`
continues to deny a finalizing run. Active provider/tool paths retain their full
Catalog access and normal bounded lineage binding; no lifecycle access is widened.

For a finalizing run this supplement also calls the body-free Catalog
`CheckCapturedRunGrantsTx` primitive. The exact captured graph must include every
ordinary ancestor, including lessons used through derived presentations or
skills. Current run/session owner, task session/revision, exact source grants,
source ACL/revision/hash and fresh task/delegation/mandate authority remain
required. A missing ancestor pin or grant denies publication. This check returns
no bodies or new pins; it is not completion or inference authority by itself.
The owning completion checks its expected constraints revision before this call.

Completed independent owner verification and outbox operations retain their own
completed-result/owner-transition proof and applicability checks; they do not
borrow current inference access or compare the old run revision with a later
owner-completed task revision. Generic AccessTx and active provider/tool paths
retain their existing lifecycle restrictions. The older general Catalog
CheckRunSourcesTx checks source ACL/revision/privacy and delegation, not ordinary
nondelegated task grants. The reusable captured-grant regression documents that
separate boundary; this change wires the extra check only for experience-bearing
final publication, pending a separate review of general publication wiring.

At finalizing completion, captured-grant validation also requires the exact
current run token. After graph, grant and owner-authority waits it repeats a
plain metadata check of session/run/lease/fence identity and
`expires_at > clock_timestamp()`. The outer completion already holds its
ownership SHARE guard; this final check acquires no new lease lock, returns no
body and does not wait behind a queued takeover. A grant wait crossing lease
expiry denies the final while retaining the attempts. Completed independent
owner/outbox operations keep their own completed-result transition proof and do
not acquire a new live inference lease through this applicability supplement.

Migration082 distinguishes content removal from publication. Exact task working
erasure is allowed under an existing deletion of that session's owner, with
unchanged task identity/authority and a single working-revision increment.
Only empty plan/actions and the exact privacy marker may be written. Extra
payload, foreign/missing deletion proof or any identity change is denied.
Currentness vetoes remain mandatory for all ordinary publication, including
after erasure. A stale copied observation must not prevent subsequent owner
deletion or replay of a later independent anchor. Any populated experience
provenance refuses downgrade: a failed first deletion can leave an external
anchor requiring replay without a committed SQL deletion row.

An installed experience store makes the active source verifier a compound owning
operation: base Catalog proof, current Broker contract lookup outside SQL locks,
then a fresh owning Catalog/experience proof. This adapter explicitly declares
the existing trusted two-second maximum for the whole operation. It preserves
invalid inherited bounds for rejection; absent the experience store, the base
bound follows the foreground source guard's default2s (or its explicit inherited
bound). Background checks and the250ms observation interval are unchanged. Parent deadlines,
cancellation and Stop still clamp the check. Final/tool/batch DATA stays held
until the fresh proof succeeds; expiry, privacy withdrawal, timeout or current
authority loss denies release and cancels the provider. Existing usage evidence
survives a denied proposal; there is no extra provider request, replay or refund.
The two-second limit is a verification ceiling, not a measured latency SLO.

The controlled comparison prepares its original tasks, bounded accepted notes
and reviewed skill in one task-data database, then freezes that database before
the48 A/B/C inputs. Each input receives an independent fresh clone; provider auth
and the persistent640-entry allocation journal remain in a separate owning store.
This is a controlled lab partition, not cross-database production budget authority.
The complete57-slot schedule precedes setup; failed/unexecuted stages and nullable
usage survive aborts. Exact owner/day history must be imported once before source
work, and copied prefix costs must not be counted again for every clone.

An independent private method/resource/status receipt may trigger the predeclared
original03 owner intervention. It is never model or lesson DATA, product effect
truth or proof of ABSENT. The original UNKNOWN/no-final remains immutable; only
the separate current authorized lookup and Reconcile prove absence. Original04
has no intervention. Compiled recipient and runtime executables form part of the
frozen environment identity. Synthetic controlled execution establishes these
mechanics only; native quality, full unique cost and independent review remain
separate release requirements.

Prospective native preparation pins the complete frozen source-file inventory,
all five compiled artifacts (recipient app, Broker, Broker control test, extension
and HTTP runtime), both corpus hashes, exact schedule/policies, environment hashes
and one recursively verified latest closed owner/day history. Runtime artifacts
are retained under the proof directory and mounted read-only; mutable compiler
cache names are not native artifact identity. A missing private parent is prepared
and its restricted permissions checked offline before auth. Changing any sealed
file, compiled artifact, history, policy, slot or finite deadline denies admission.

The source imports only the latest inclusive current-day charge using one ordinary
owner-day reservation; its immutable receipt is also bound to the independent
global journal before source execution. A cloned old reservation is counted once
by exact reservation ID and canonical row. Altered duplicates or reused operation
IDs deny the cost union. Mutable cloned scope Used projections are not summed.
Aggregate carry rows, one-turn model reservation rows, provider-method entries,
reported usage and unknown invoice cost remain distinct measurements. Neither an
admitted attempt without a provider entry nor NULL usage is treated as a refund.
