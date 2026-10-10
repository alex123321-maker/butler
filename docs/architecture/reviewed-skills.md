# Reviewed procedural skills

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

## Release status

Unit/lifecycle checks passed on isolated PG, including MaxConns=2 concurrent
loads, stale fence/revision, interface change, rollback/revoke and immutable
history. Full release still requires catalog-owned derived-source deletion and
restore integration, plus an actual extension procedure across durable approval
and restart, followed by independent outcome verification. Operator flags remain
disabled until those pass.

The reference [Hermes skill discovery](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills/)
uses short discovery before selected detail. Butler preserves immutable versions,
independent source proof and explicit authority/deletion boundaries.
