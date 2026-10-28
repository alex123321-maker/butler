# External effect journal

Tool Broker owns the PostgreSQL journal for dispatch and external evidence.
Orchestrator remains the run authority; runtimes execute one approved proposal.
A technical runtime response, a model's statement, memory update, final answer,
notification or provider session is never proof of an external write.

Browser revision 3 read/open/navigate are enforced GET/HEAD observations and use
`read_result`. `browser.prepare_fill/prepare_type` alone support
`local_browser_preparation_v1`: pinned session, exact current URL, newer version,
success and persistent frozen-network acknowledgement prove only browser-local
preparation. The private policy is derived from the exact approved call. Generic
live fill/type/click cannot borrow this verifier. DOM submission acknowledgement
remains unknown; the budget root's unknown-effect stop remains in force. No
automatic retry or external recipient idempotency is inferred from local state.

Revision1 `browser.submit_get_form` is a separate enforced GET observation,
not an input gesture or form POST. Its two-phase frozen compiler rejects
credential/hidden/password/file fields and rechecks live state before GET
navigation. Its `read_result` evidence describes that read only. A previous
unknown write and consumed grant remain unchanged; later mutations still deny.
See the browser session contract for bounds and unsupported form behavior.

## Identity and authorization

`PrepareToolCall.logical_action_ref` is optional private service metadata. The
accepted-input repository allocates a random slot **before** preparation and
saves it under `checkpoint_json.action_slots`. It binds run/call, task revision,
constraints revision, tool and canonical argument hash. Replacing
`checkpoint_data` preserves these slots. Approval continuations retain the slot
and permanent effect ID; recovery checks both against the saved state.

Two new proposals with identical arguments receive different random slots.
Cross-run reuse requires the repository's explicit trusted `LinkActionSlot`
operation. Its parent slot must have the same owner, task, task/constraints
versions, tool and argument hash. No model-facing API selects a parent slot.
Without a logical reference, protection is explicitly limited to the existing
run/tool-call identity.

The permanent journal ID hashes owner/task/slot. A separately stored immutable
fingerprint pins the complete tool contract hash (including contract revision,
effect policy and runtime target), arguments, resource, owner, task/constraints
versions and credential metadata. Changed fingerprints cannot replace an intent.
Execution approvals/grants still bind the exact run/call/action hash. A logical
reference alone grants no execution permission.

## States and the irreversible boundary

The journal states are `prepared`, `allowed`, `executing`, `effect_confirmed`,
`effect_absent`, and `effect_unknown`. Grant states continue to distinguish
technical `dispatch_completed` from uncertain dispatch.

Claiming a grant uses one PostgreSQL transaction. It takes a shared live session
ownership lock, then run/task locks, verifies the pinned/current revisions,
owner, constraints, active task and execution policy, then commits the grant,
journal `executing` state and dispatch record. Owner cancellation/amendment takes
the conflicting ownership lock first. The shared locks are released before the
runtime call. A stop committed first denies dispatch; a stop after dispatch
cannot undo a request that may already have reached the recipient.

`prepare_only` permits only verified read effects, including when an earlier
approval or broader mandate exists. Expiry checks at commit use PostgreSQL wall
clock, rather than the transaction start time.

Grant completion, journal state and append-only evidence commit together. Lost
responses, failed writes, missing results and unsupported evidence leave an
uncertain, spent intent. A repeated Broker RPC returns the same uncertain effect
metadata and never calls the runtime again. Recovery may mark an interrupted
`executing` record unknown; this never renews its grant.

An audited abandonment of a failed taskless local communication prepare does not
reconcile that journal entry. It can qualify an independent later owner Input's
fixed credential-free `communication.message` publication through the loop gate:
all conflicting unknowns must be failed local message/widget preparations with
matching original Input/owner/session and explicit per-effect acknowledgement.
The old revision1 contract must declare `local_runtime_ack`; an observed runtime
acknowledgement or an absent/confirmed effect is not inferred from that policy.
The old Input/run must be failed, its obligation closed with failure and its
failure/uncertainty audits present. Same-Input retries, executing records,
external effects and incomplete provenance remain blocked. This read-only
qualification changes no journal, grant, reservation, delivery or verification
state; ordinary exact approval/current source/destination checks still apply.

## Declared recipient contracts

Tool contracts declare `contract_revision` and `effect_contract`. Defaults are
`retry_safety: unsafe` and `verification_method: none`; ordinary HTTP, browser
interactions and Telegram sends receive no external idempotency guarantee.
Read tools use `read_only`/`read_result`. An explicitly declared local preparation
may use `local_runtime_ack`; it cannot prove an external write.

The supported external protocol is deliberately narrow:

```json
{
  "retry_safety": "external_idempotency",
  "verification_method": "external_receipt_v1",
  "idempotency_header": "Idempotency-Key",
  "scope_origin": "https://recipient.example",
  "verification_url": "https://recipient.example/effects/{idempotency_key}"
}
```

Only `http.request` mutations may use this policy. The operator must have an
actual recipient guarantee for the exact origin; generic acceptance of a header
is insufficient. `X-Idempotency-Key` is also supported. The key derives from the
permanent journal ID and fingerprint. Broker passes it and the fingerprint via
private runtime execution context. Runtime injects the fixed key header and
`Butler-Action-Fingerprint`, rejects caller overrides, and disables redirects
for these mutations. It performs no implicit retry.

A successful, complete 2xx body must contain:

```json
{
  "butler_effect_receipt": {
    "idempotency_key": "<the supplied system key>",
    "action_fingerprint": "<the supplied fingerprint>",
    "effect_state": "effect_confirmed",
    "artifact_id": "<recipient's actual result identifier>"
  }
}
```

A generic 201, `Location` header, unrelated ID, truncated body, different key or
fingerprint cannot confirm the effect. The production catalog declares no
generic external guarantee; the contracted fixture proves this protocol only
for a receiver that actually implements it.

## Independent reconciliation and provenance

An uncertain effect is reconciled by a **separate approved read** to its pinned
verification URL through Broker. The read must already have a confirmed journal
observation from trusted runtime output, with matching owner/task, execution
action/run/call and result digest. Model text or a fabricated verifier result
cannot substitute for that captured observation.

The lookup returns the same receipt envelope. `effect_confirmed` requires the
matching key/fingerprint and non-empty artifact identifier. `effect_absent`
requires the matching key/fingerprint and no artifact. Only an independent
lookup can establish absence; a mutation's response cannot. Reconciliation
preserves the original observation timestamp and records its own capture time.
PostgreSQL rechecks provenance before appending the evidence and changing state.

Evidence contains source action/run/call, observation method, verdict, timestamps
and SHA-256 result/artifact digests. It does not copy raw bodies, artifact IDs,
cookies, tokens or credential values. Evidence rows reject update/delete.
Read outcome digests are computed after existing Broker secret redaction.

Private `GET /internal/action-effects?run_id=...` returns only typed
`action_id`, `effect_state`, `evidence_refs`. POST accepts `action_id`, an
`observation_authorization_id` and the already captured redacted `result_json`.
It requires the private service token and rechecks the captured digest. It is
absent from the model tool registry. `PostgresStore.GetRunEvidence` supplies the
same typed bridge to final-result coordination without direct Broker SQL.

## Retry, compensation and limits

There is no automatic replay of an uncertain mutation, even under an external
key contract. Independent evidence informs the owner; observed absence does not
silently renew a spent grant. A further attempt or compensation is a new intent
with its own exact authorization. Identical arguments do not grant permission
or identify such a new intent.

Exactly-once effects for arbitrary external services are not claimed. A browser
click or generic Telegram send without recipient evidence remains unknown and
blocks model continuation. A downstream memory, artifact, notification or final
answer failure cannot turn a confirmed or uncertain journal entry into an unused
permission. Local browser contexts can still disappear after process loss.

Migration 035 creates the journal, dispatch history and immutable evidence.
Legacy completed dispatches migrate to **unknown**, never confirmed. Untouched
legacy intents can upgrade only while their exact old binding still matches.
Migration 036 supplies the task policy checked at dispatch. Operational rollback
must preserve journal data; dropping these tables removes durable evidence and
must never be treated as permission to repeat earlier effects.

## Reproducible validation

The isolated lab PostgreSQL suites cover concurrent grant CAS, commit failure
before dispatch, actual connection loss after recipient acceptance, failure
persisting a received result, service restart, immutable evidence, independent
confirmed/absent lookup, invented verifier output, cross-run explicit references,
separate compensation approval, cancellation/constraints/owner/policy races,
scoped system header injection and redirect denial. Accepted-input tests check
pre-Prepare persistence, approval recovery and checkpoint replacement. See
`deploy/effect-journal/verification.json` for commands and results. These local
fault fixtures do not prove idempotency or verification for unrelated websites.
