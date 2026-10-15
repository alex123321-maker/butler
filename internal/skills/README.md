# Reviewed procedural catalog

This Broker-owned module stores version digests, independent validation,
activation generations, correction grants and run usage pins in PostgreSQL.
Procedure text lives only in the deletion-aware memory catalog as immutable
derived DATA. Its actual verified final-message parent retains all original
source dependencies. Deletion physically redacts derived text; history keeps
content-free identities. A missing or restored stale deletion anchor denies reads.
Skill prose is procedural DATA. It does not replace the operator prompt, create
tool permissions, resolve credentials, execute scripts or fetch package URLs.

The optional fixed tools `skills.search`, `skills.load` and `skills.propose`
require ordinary Broker preparation, current task authority, admission, budgets
and effect journaling. Search returns at most 12 descriptions. Load returns one
exact digest, at most 16 KiB, with at most four pins per run. Proposal cannot pass
a caller-supplied success verdict to validation.
Agent proposals reference an exact currently accessible catalog source containing
candidate manifest JSON. They cannot put prose into immutable tool action args;
the original proposal is retained as a deletion dependency. The owner staging
API accepts raw JSON. Live observations carry procedural DATA, while Orchestrator
stores content-free references and rehydrates current text on recovery.

The private source verifier reads owner-verified task results and actual
confirmed action journal evidence through owned APIs. Both source evidence and
the current catalog must match each dependency's contract hash and revision.
The fixed v1 validator checks stop/reconcile behavior and compatibility. Its
report is not evidence that arbitrary natural-language steps ran automatically;
an actual independently verified source trial supplies that evidence.

Owner operations use the `/skills` page, `/api/v4/skills` and the owner-control token. The backend
relay uses separate service authentication and a fixed private endpoint. Versions
progress candidate → tested → active. Changing the active pointer retires the
old version; loaded runs remain pinned until revoke or incompatible dependencies.
Permanent revoke cannot roll back. Usage/quality keeps exact task/run references.

Automatic activation is limited to `descriptive_correction_v1`: an owner's
unexpired exact parent-digest grant and current assigned owner mandate must agree.
Only description/step intent wording may differ. Inputs, source proofs, dependent
tools, step order, checks and retry/approval rules remain exact. Other proposals
await owner review. A procedure cannot grant capabilities by its wording.

Enable deliberately after migration 051, the shared persistent deletion anchor
and private service authentication are configured: `BUTLER_SKILLS_ENABLED=true`
on Broker and `BUTLER_SKILL_CONTROL_URL=http://tool-broker:8081/internal/skills`
on Orchestrator. Defaults remain disabled. Database settings cannot redirect the
fixed private relay. Source/result evidence is served only on the authenticated
internal endpoint; the owner token never reaches the model or runtime.

`TestReviewedSkillRealBrokerDurableApprovalE2E` runs the production Broker and
separate reviewed runtime, actual PostgreSQL, fresh Orchestrator/provider
instances at each approval, recipient receipts and owner-verified outcomes.
Inference is scripted proposals; this tests control flow, not model planning
quality. PG tests cover concurrent pins, incompatible updates, rollback,
descriptive correction/revocation, physical source deletion and backup restore.
See the [acceptance matrix](../../docs/architecture/reviewed-skills-acceptance.md).

Experience-derived proposals (#90) optionally declare exact `lesson_sources`.
The private owning verifier also inspects complete parent graphs when that field
is omitted. Any experience lineage requires at least two distinct independently
owner-verified task/run originals and current accepted compatible lessons, with
no failed/unknown action qualifying as a successful recipe. Lesson/original
parents join the existing deletion-aware manifest graph. Legacy evidence without
experience lineage retains its existing validation; stage/test/owner review/CAS,
rollback, approval and tool authority remain unchanged. The experience consumer
and quality gate remain off; see
[the contract](../memory/experience/README.md).
