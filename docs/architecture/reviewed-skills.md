# Reviewed procedural skills

Experience promotion (#90) adds optional exact `lesson_sources` to the immutable
manifest. The private owning verifier derives any experience lineage from the
complete source graph even when omitted by the caller; that path requires at
least two distinct independent owner-confirmed task/run originals and current
accepted compatible lessons without failed/unknown steps. The validated lesson
parents join the existing Catalog-derived manifest source. Existing legacy
evidence remains supported, and owner test/review/CAS activation and rollback
remain mandatory. See [typed episodic experience](episodic-experience.md).

A skill is an immutable, versioned procedure with bounded descriptions,
applicability/inapplicability, inputs, exact dependent tool hashes, explicit
steps, stop/reconciliation rules and independently verified task sources. It is
neither a memory fact nor an executable tool. Its URI is provenance; Butler does
not fetch a package URL or execute a mutable skill directory.

## Ownership and admission

Tool Broker owns the catalog. Orchestrator's private typed evidence reader owns
task/result/transcript inspection; budget and effect modules supply read APIs.
PostgreSQL is authoritative. Skills cannot set system instructions, select an
owner/mandate or create credential/egress authority. All actual tools retain the
same Broker, task, approval, budget and effect checks.

Kernel catalog tools may use an exact local resource scope `tool:skills.load`
with an empty path prefix. Mandate tool name and effect must also match.
Wildcards, prefixes, queries and URL/path substitutions are rejected. This scope
grants neither network access nor extension installation.

Discovery supplies at most twelve descriptions as private Document DATA. The
model loads a selected digest through `skills.load`; procedures are omitted until
selection. Load enforces bytes and four run pins. Source prose cannot change
actual tool declarations.

## Evidence and lifecycle

The fixed v1 validator requires owner-verified criteria/results and actual
effect-confirmed actions using exact historical contracts. A model final or
successful transport status is insufficient. Mutation procedures stop/reconcile;
only read contracts can recommend re-observation. Supplied test descriptions are
documentation, not caller-supplied pass flags.

Lifecycle is candidate → tested → active → obsolete/revoked. An incompatible
candidate never replaces a working version. Activation uses generation CAS;
rollback is explicit owner review with compatibility revalidation. Revoke is
permanent for the digest. Old pins may use obsolete versions after an update,
but cannot use revoked/incompatible ones. Provider release, live events and
recovery recheck pins. Dispatch verifies pins/dependencies in its owned
transaction after source guards and before effect/budget/grant commit. Final
admission checks skill lifecycle and dynamic installation state too.

Ordinary approval continuations remain Butler-owned. Skills do not invent a
second workflow engine or reuse provider function responses after restart.
Known observations remain DATA; unknown effects remain non-replayable.

An owner may authorize descriptive correction of one exact active parent under
a current mandate/revision/purpose and expiry. The bounded class retains every
dependency, input, check, source, step order, retry and approval requirement.
New procedures/capabilities need a new owner decision.

Quality reads verified results, known failed tool observations, durable model
attempts, conservative token allowances and nullable observed usage alongside
source trials. Unverified attempts stay separate. Missing observations do not
become zero billed cost; created-skill counts are not success metrics. Quality
cannot promote a skill automatically.

## Source privacy and publication

`skill_versions` stores digest, immutable dependency identities, correction shape
and an exact catalog source reference, with no procedure prose. The private
evidence reader binds a verified task result to its actual immutable final run
and assistant message. The catalog expands that message's original source graph;
citations cannot omit dependencies or authorize another scope. Stage requires a
current owner or owned run/job proof for the complete graph. Same-owner sources
in different scopes still require access to every parent.

The raw reviewed manifest is a catalog-derived `model_candidate` DATA source.
Model `skills.propose` arguments carry an exact current catalog `source_ref`,
not manifest prose: the agent can emit candidate JSON as an actual source/result
and propose its reference in a follow-up owned task. Current access to that
proposal and all verified result parents is required; both remain deletion edges.
Owner staging accepts raw reviewed JSON through the authenticated control API.
Live load/search observations contain authorized DATA, but durable transcript,
artifacts, scratch and prepared approval checkpoints retain digest/source refs
only. Fresh recovery resolves current text rather than replaying saved prose.
Deleting an original message/source recursively redacts the procedure body;
historical version hashes, usage and lifecycle events remain inspectable.
Restoring an older database cannot recover readable procedure text before
deletion-anchor reconciliation. Missing anchors and missing graph edges deny
publication or loading. Validation stores content-free owner-evidence digests.

Loads pin exact source and skill versions. Source guards run before lifecycle
and dynamic dependency guards in provider release, irreversible Broker admission
and atomic final-result admission. Revoked skills cannot resume or dispatch;
obsolete pins may continue only with current sources and compatible contracts.
The source graph union is locked in catalog order before correction lifecycle
locks. Current grant, mandate and database time are checked after lock waits.

## Operations and evidence

Migration 051 and the persistent memory deletion anchor are required. Broker's
`BUTLER_SKILLS_ENABLED` and Orchestrator's fixed private
`BUTLER_SKILL_CONTROL_URL` are explicit operator opt-ins. The authenticated
`/skills` page stages raw manifests, shows test/lifecycle history and usage,
reviews measured outcomes and revokes bounded correction grants. It retains the
owner token only in page memory. Source deletion leaves metadata history without
serving procedure text.

The [acceptance matrix](reviewed-skills-acceptance.md) records actual PG,
production Broker/recipient approval/restart and UI tests and their limits.
The fixture does not establish a reduction in inference cost or real model
planning errors; its measured extra load turn remains visible in quality data.

The reference [Hermes skill discovery](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills/)
uses short discovery before selected detail. Butler preserves immutable versions,
independent source proof and explicit authority/deletion boundaries.
