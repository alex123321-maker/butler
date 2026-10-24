# Durable execution budgets

Migration098 keeps existing Inputs at context_epoch0. New verified explicit
taskless Inputs use input:<InputID> as conversation subject with acceptance+24h
deadline. Rerun/restart/approval resume reuse spending; independent owner requests
have independent finite allowance. Root/owner-day/mandate/delegation guards stay.


Operator `disable_conversation_model_turns:true` removes only the session/day
model-turn ceiling. Charges continue accumulating against the unchanged scope;
re-enabling enforces its original limit and total spending. Local and provider
inference use the same setting. Other dimensions, all other scope kinds, absolute
deadlines and overflow checks remain enforced. Core/Broker configuration must
match; the owner API exposes `conversation_model_turns_enabled`. No ledger reset,
refund or large substitute limit is used.

Explicit operator `disable_owner_day:true` in the shared budget policy disables
the aggregate owner/day scope for new admissions, including preflight and local
auxiliary calls. Default behavior keeps the guard enabled. Existing daily history
is immutable and is reused when re-enabled. All task, conversation, mandate,
delegation and job/policy bounds and execution guards still apply. The owner read
API reports `owner_day_enabled` alongside its historical scopes and attempts.

Generative memory jobs reuse `ReserveAuxiliaryTx` with a trusted `ModelQuote` and
an additional finite policy scope. Job/policy/root/mandate/owner-day counters are
initialized and locked together in canonical order. The job owner validates
current authority/locality, persists each attempt linked to the reservation and
clamps dispatch to every scope/claim/authority deadline. The existing no-refund
contract remains: actual usage and conservative reserved capacity are distinct;
missing usage is unknown. No second budget ledger is introduced. See
[background memory jobs](../../docs/architecture/background-memory-jobs.md).

The trusted consolidation job owner may additionally supply at most eight current
root task/mandate identities after proving its explicit owner context pins. The
controller resolves every existing scope and reserves one full quote against their
deduplicated union plus job/policy/owner-day scopes in the same transaction. A
single exhausted/changed scope rolls back all charges and initializations. No
legacy caller gains another scope, and no model argument can select these roots.

This Go package owns migration037's PostgreSQL policies, conservative reservations,
model attempts and normalized loop observations. Production Orchestrator, Broker
and Task Store bootstraps install its controller.

Root task, assigned mandate and owner UTC-day scopes share counters across runs,
children and amendments. Conversation uses session/day plus owner/day without a
separate planner or critic. Identity comes from durable run/session/task ancestry.
Conversation repetition additionally distinguishes original Input events using
the permanent effect/run provenance. Independent owner requests can repeat a
catalog read; retries, answer resumes and replacement runs for the same event
retain its loop limit. Existing observations are read in place, with conservative
fallback for missing provenance. Task-tree loops, session-wide uncertain writes,
finite conversation/day budgets and all retained spending stay unchanged.
An independent taskless owner may ask for a canonical protected credential
interaction after an acknowledged failed local communication prepare. The
Broker-derived canonical-contract flag additionally requires an exact live
Input/intent and the complete old uncertainty acknowledgement audit. This cannot
authorize secret release or browser preparation. External/executing uncertainty,
missing provenance and finite allowance exhaustion still deny; old evidence and
spending are neither cleared nor reclassified.
An owning AuthorityResolver checks current assignments on the same transaction
and returns immutable mandate limits/deadline. Model arguments cannot supply
scope, owner, quote, reservation ID, monetary rates or authority.

The resolver also returns a transient AuthorityDeadline from the current goal,
purpose policy and active-hours window. Model/child admission rechecks it on the
same transaction after budget contention. It clamps release contexts and never
replaces persisted scope Deadline or replenishes spent counters.

Concurrent scope and reservation initialization handles both canonical IDs and
their owner/subject or owner/operation unique keys. Existing rows must still match
the canonical ID and complete immutable policy/fingerprint. A conflict never
updates limits, resets spending or charges a replay a second time.

Eight finite counters cover turns, tokens, monetary micros, external calls,
children, active milliseconds, tool egress bytes and provider input bytes. Zero
is a hard limit. Default root/conversation/mandate policy permits 64 turns,
67,108,864 token allowance, 128 calls, 8 children, one hour reserved active time,
128MiB tool egress, 256MiB provider input and zero monetary charge. Owner/day limits
are four times these values. Root deadline is seven days from original creation;
conversation/day ends at midnight UTC. Per model/tool reservation is 120 seconds.
Runtime contexts use the earliest reserved time or applicable absolute deadline.

BUTLER_EXECUTION_BUDGET_POLICY_JSON is deployment operator configuration, identical
in Orchestrator and Broker. Omitted fields retain finite defaults. Model cost basis
defaults to unknown and stops inference until the exact provider/model declares
operator_rate (nonzero integer micros per token), subscription_no_overage, or
local_no_billing (both zero rates). A basis is not proof of entitlement or credit
purchase permission. Native Antigravity sends no credits and has no model fallback.
For the explicitly selected Google AI Pro subscription, an operator can configure:

```json
{"models":{"antigravity:gemini-3.8-flash-tiered":{"basis":"subscription_no_overage"}}}
```

EnsureTx and ReserveTx share the caller's connection. Sorted scope locks and an
owned savepoint prevent partial spending and leaked reservation placeholders.
Broker reserves effect:<permanent EffectID> before journal dispatch commit; Task
Store reserves child:<system child ID> before insertion. Each actual model attempt
gets a fresh committed random operation ID even after recovery. Live ownership,
run/task and mandate/goal locks precede budget locks. Unknown effects, provider
failure and crashes never refund allowance. Actual provider usage stays separate;
missing observations remain null. Immutable policies cannot be reset by a revision
or changed defaults. Down refuses any scope, reservation, attempt or loop history,
including an unspent zero-limit policy.

Loops bind root, pinned task/constraints revisions, exact tool/contract and
canonical arguments. Equivalent finite numbers/object order compare without float
rounding. Typed errors ignore free-text wording; successful observations strip
only known volatile metadata. Three matching observations stop the next call.
Unknown non-read effects block new non-read actions throughout the tree, including
a different tool. Independent reads can reconcile; compensation is a new action.

Stops persist execution_stopped, a concrete reason and preceding continuation,
block task/input and cancel transport. They do not create a verified final result.
Atomic finalization retains artifact/criteria/evidence and model-only results stay
unverified. A malformed runtime output schema or completed result carrying an
error cannot become confirmed evidence. Batches advertise false and fail entirely
before the first tool executes; no partial result set continues the model.

Native Antigravity currently declares conservative limits: 1,048,576 context plus
8,192 output tokens reserved per attempt. Other adapters without declared limits
fail closed. Private transport.WithInputByteLimit checks the complete native
serialized history, system input, schemas and results before HTTP. Unset cap is
8MiB; tool egress reserves 4MiB per call. Actual wire/session egress enforcement and
other cloud adapter byte guards belong to issue31; a reservation alone does not
prove an unchecked runtime obeyed its quote. Assigned local_only denies the
currently supported cloud provider.

GET /api/v3/execution-budgets requires owner proof and reports the proved owner's
latest scope/attempt observations. Caller owner IDs are ignored. Read data cannot
mutate policy or replenish counters. Reproducible lab PostgreSQL/race checks and
current acceptance boundaries are in deploy/execution-budgets.

`CheckRootAdmissionTx` is an owned initiative promotion preflight on the caller's
transaction. It checks declared model cost/limits and current shared scope
capacity without creating scopes or charging a model turn. Actual inference
still reserves atomically; later depletion cannot authorize a network call.

`CheckScheduledRootAdmissionTx` retains that behavior for assigned agent jobs.
An exactly approved schedule without a broad mandate checks the ordinary finite
root/day scopes only. It does not create authority/scopes/reservations or reset
allowances. Actual inference and tools still reserve through the same controller;
the schedule additionally retains a finite immutable maximum and expiry.

`AdmitLocalModel` is a separate live query/rank boundary. Its private typed
`LocalTarget` is fixed to the composed internal service at local-models:9090;
requests cannot choose endpoint/model/locality. Exact local profile pricing must
explicitly declare `local_no_billing`, independently of locality/permissions.
Current lease, unfinished run, active pinned task and current goal/mandate privacy
precede source snapshot checks and root/mandate/UTC-day allowance rows. local_only
and cloud_allowed support this local path; unknown policies deny. Cloud admission
is unchanged and cannot treat local pricing as permission to leave the network.

Each actual local inference gets a fresh reservation plus a finite auxiliary
scope named live:<runID>; root/day/mandate spending persists across new runs and
children. 2048 tokens per item plus one template/query allowance is conservative
reserved capacity, not measured usage. Source/authority are rechecked after budget
waits and every250ms during IPC. `CheckLocalAuthority` rejects late results without
charging a second turn. Indexing uses the separately claimed durable auxiliary
job contract. No completed lease, failed attempt or changed revision refunds
allowance or resets an existing immutable scope.

`CheckTaskContinuationTx` is a read-only prospective check inside the owning055
transaction. It resolves current authority and immutable finite scopes, then
checks a trusted declared next-turn quote after waits without creating scopes or
reservations. It grants no inference right; actual provider admission still charges
once through the existing model ledger.

`AdmitOwnerQuery` is a separate authenticated Web search boundary. It admits only
one pinned local embedding query (2048 bytes/4096 reserved token units/30s), with
no run/task or source-body release. Each attempt reserves a fresh request scope
and the shared fixed12-attempt UTC-minute scope through the existing auxiliary
ledger, plus owner/day when enabled. The minute cap remains when owner/day is
disabled. Restore proof runs before/after reservation; the HTTP context and
earlier reservation deadline bound RPC. There is no automatic retry or refund.
