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

The owning memory.facts path applies the same current lesson-ancestor and exact
trusted exposure checks before reading accepted snapshots. Owner acceptance of
a derived episodic/profile value does not remove the lesson's environment, tool
contract or negative-feedback conditions. Legacy facts with no lesson ancestry
retain their ordinary authority checks.
