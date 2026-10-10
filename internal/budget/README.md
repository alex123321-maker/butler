# Durable execution budgets

This Go package owns migration037's PostgreSQL policies, conservative reservations,
model attempts and normalized loop observations. Production Orchestrator, Broker
and Task Store bootstraps install its controller.

Root task, assigned mandate and owner UTC-day scopes share counters across runs,
children and amendments. Conversation uses session/day plus owner/day without a
separate planner or critic. Identity comes from durable run/session/task ancestry.
An owning AuthorityResolver checks current assignments on the same transaction
and returns immutable mandate limits/deadline. Model arguments cannot supply
scope, owner, quote, reservation ID, monetary rates or authority.

The resolver also returns a transient AuthorityDeadline from the current goal,
purpose policy and active-hours window. Model/child admission rechecks it on the
same transaction after budget contention. It clamps release contexts and never
replaces persisted scope Deadline or replenishes spent counters.

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
