# Reviewed procedural catalog

This Broker-owned module stores version digests, independent validation,
activation generations, correction grants and run usage pins in PostgreSQL.
Skill prose is procedural DATA. It does not replace the operator prompt, create
tool permissions, resolve credentials, execute scripts or fetch package URLs.

The optional fixed tools `skills.search`, `skills.load` and `skills.propose`
require ordinary Broker preparation, current task authority, admission, budgets
and effect journaling. Search returns at most 12 descriptions. Load returns one
exact digest, at most 16 KiB, with at most four pins per run. Proposal cannot pass
a caller-supplied success verdict to validation.

The private source verifier reads owner-verified task results and actual
confirmed action journal evidence through owned APIs. Both source evidence and
the current catalog must match each dependency's contract hash and revision.
The fixed v1 validator checks stop/reconcile behavior and compatibility. Its
report is not evidence that arbitrary natural-language steps ran automatically;
an actual independently verified source trial supplies that evidence.

Owner operations use `/api/v4/skills` and the owner-control token. The backend
relay uses separate service authentication and a fixed private endpoint. Versions
progress candidate → tested → active. Changing the active pointer retires the
old version; loaded runs remain pinned until revoke or incompatible dependencies.
Permanent revoke cannot roll back. Usage/quality keeps exact task/run references.

Automatic activation is limited to `descriptive_correction_v1`: an owner's
unexpired exact parent-digest grant and current assigned owner mandate must agree.
Only description/step intent wording may differ. Inputs, source proofs, dependent
tools, step order, checks and retry/approval rules remain exact. Other proposals
await owner review. A procedure cannot grant capabilities by its wording.

Full acceptance remains pending: derived-source deletion/restore integration and
the actual Broker procedure/approval/restart scenario are under implementation.
Keep `BUTLER_SKILLS_ENABLED=false` and `BUTLER_SKILL_CONTROL_URL` unset until
combined acceptance passes. Isolated unit/PG checks are not the full release gate.
