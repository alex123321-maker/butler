# Execution budget contract

Budget Service owns migration037's immutable scope policies, spent reservations,
model attempt IDs and normalized loop observations. PostgreSQL is authoritative.
Orchestrator requests conservative admission before each Start/Continue/Submit
model turn; Broker reserves allowance in its permanent effect-claim transaction;
Task Store reserves children before insertion. A model cannot supply scopes,
quotes, reservation keys, rates, ownership or mandate authority.

Applicable scopes are root task, current assigned mandate and owner UTC calendar
day. Conversation uses session/day and owner/day without planner/critic overhead.
Restart, worker/run ID change, delegation and amendments do not reset root or
mandate spending. Daily counters change at midnight UTC; root/mandate counters
continue. Child limits may restrict these scopes further, never replace them.

Finite counters cover turns, conservative tokens, monetary micros, calls,
children, reserved active milliseconds, tool egress and provider input bytes.
Zero is a hard limit. Absolute deadlines bound elapsed work; each model/tool
context uses the earliest reservation or scope deadline. Native Flash reserves
its declared 1,048,576 context and 8,192 output tokens instead of guessing exact
serialized token counts. Observed usage is separate; missing usage is null.

Operator BUTLER_EXECUTION_BUDGET_POLICY_JSON must match in Orchestrator and Broker.
Default model costs are unknown and deny inference. Exact provider/model basis is
operator_rate with nonzero integer rates, subscription_no_overage, or
local_no_billing. Subscription entitlement, incremental price, availability and
API billing permission are distinct. No price is invented and no credit purchase
or stronger-model fallback follows from this configuration. Native Antigravity
sends no credits and retains Gemini3.8 Flash. An assigned local_only mandate denies
the currently supported cloud adapter.

Ownership/session/run/task locks precede current mandate/goal authority checks,
then sorted budget scope locks. The owning actionauth adapter validates assignment
and supplies immutable mandate limits/deadline on the same transaction. Budget
code reads trusted identity but writes only its own tables. EnsureTx/ReserveTx
never take another connection. An owned savepoint rolls back partial charges and
reservation placeholders without discarding other caller work.

The assigned authority adapter also supplies a transient AuthorityDeadline from
the current mandate, goal, confirmed purpose-policy expiry and active-hours end.
It is separate from the original immutable scope Deadline. Model/child admission
and effect Claim recheck the DB clock after budget waits, retaining the same held
authority rows, and reject expiry before permission is spent. Model/runtime
contexts use the earlier bound; a model context already expired is refused before
transport. Owner shortening a mandate deadline does not reset its allowance.

System operation identity is permanent EffectID for tools, durable child ID for
children, and fresh committed random ID for every actual model attempt. Replaying
a quote is idempotent; changing scope or charge conflicts. Reservation replay is
not dispatch permission: the effect journal independently rejects spent/unknown
effects. Crashes, timeouts and unknown outcomes do not refund allowance.

Loops normalize root, pinned task/constraints revisions, exact tool/contract and
canonical arguments across restart. Equivalent numbers/object order compare
without floating point loss. Typed error categories ignore messages; success
observations strip only known volatile metadata. The next call after the repeated
observation limit stops with repeated_action_without_progress. Unknown non-read
effects throughout the task tree block new non-read calls even with a different
tool. Independent authorized reads may reconcile. Compensation needs a new
explicit authorized action; no unsupported exactly-once guarantee is claimed.

Stops persist execution_stopped with a concrete reason and prior continuation,
block durable task/input and cancel transport. They do not finalize successful
work. Frozen finalizing recovery saves its frozen result without another model or
effect call. Atomic completion stores required artifact/criteria/evidence and
keeps model-only completion unverified. Invalid runtime output schema or completed
response carrying an error remains uncertain with no automatic replay.

Batch capability is false. An unexpected batch is rejected entirely before first
dispatch, so a new iteration cannot treat partial correlated results as complete.
Current declared inference limits exist only for native Antigravity; other
adapters fail closed. Private transport.WithInputByteLimit checks the complete
native serialized request including retained history and schemas before HTTP;
unset cap is 8MiB. Broker reserves 4MiB tool egress. Lower tool quotes require a
private trusted enforced cap via budget.WithToolEgressAllowance. Runtime wire/
session egress and other cloud adapter byte guards are issue31's separate work.

Authenticated GET /api/v3/execution-budgets exposes the proved owner's latest
scopes and attempts, reserved allowances and observed actual usage. It cannot
select another owner, mutate policy or replenish counters. Down migration refuses
any durable scope/reservation/observation, including unspent zero-limit policy.
Lab checks and supported acceptance limits
are recorded in deploy/execution-budgets.

Initiative promotion may call `Controller.CheckRootAdmissionTx` after current
assigned authority checks, on the same transaction. The read-only preflight
rejects currently exhausted root defaults/mandate/day capacity and unknown model
cost/limits without charging inference. Only an actual request increments model
turns; capacity consumed after promotion still blocks that later request.
