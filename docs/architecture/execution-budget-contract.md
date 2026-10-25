# Execution budget contract

Migration098 pins context epoch1 on new Inputs; existing rows remain epoch0.
For verified explicit taskless communication at epoch1, conversation allowance
is keyed by immutable accepted Input ID and expires at acceptance+24h. Separate
owner requests have separate finite allowance; restart/approval/resume/rerun of
the same Input retain spending. Core/Broker derive identity from durable joins,
never model args. Old session/day reservations, unknown usage and limits remain
unchanged. Task/mandate/delegation and enabled owner/day scopes still apply.


The budgeted transport emits one structured numeric `model turn usage observed`
log record for each complete observation, including native total tokens. This
preserves High's thinking consumption for retained operator comparison logs;
the existing SQL input/candidate columns alone cannot reconstruct that total.
Missing observations and historical totals remain unknown. No raw model content
or credential is logged, and observations never refund reservations.

Budget Service owns migration037's immutable scope policies, spent reservations,
model attempt IDs and normalized loop observations. PostgreSQL is authoritative.
Orchestrator requests conservative admission before each Start/Continue/Submit
model turn; Broker reserves allowance in its permanent effect-claim transaction;
Task Store reserves children before insertion. A model cannot supply scopes,
quotes, reservation keys, rates, ownership or mandate authority.

Applicable scopes are root task, current assigned mandate and, by default, owner
UTC calendar day. Conversation uses session/day and, by default, owner/day without
planner/critic overhead. Operator policy `disable_owner_day:true` in
`BUTLER_EXECUTION_BUDGET_POLICY_JSON` omits the shared owner/day scope from new
model, tool, child, local retrieval, auxiliary and prospective initiative/schedule
admissions. Orchestrator and Broker must use the same setting. Root, conversation,
mandate, delegation, job/policy limits, authority and deadlines remain mandatory.
Existing daily scopes/reservations/observations are retained unchanged; enabling
the guard again reuses the same immutable daily allowance and spending. No large
sentinel limit or ledger reset represents the opt-out. The authenticated budget
read model includes `owner_day_enabled` so historical scopes do not imply current
enforcement. This operator setting cannot be supplied through model arguments.
The separate operator flag `disable_conversation_model_turns:true` disables only
the model-turn comparison on session/day conversation scopes. Core and Broker
must agree. Existing scope policies/deadlines remain immutable and all new local
and provider calls still reserve their full charge, including model turns; NULL
observations and old reservations are retained. No sentinel, refund, new scope or
reset implements this opt-out. Tokens, cost, bytes, time, external calls, all
other scope kinds and overflow checks remain mandatory. Re-enabling immediately
enforces the original stored limit against all accumulated spending. The owner
read model reports `conversation_model_turns_enabled` independently of historical
limits. This flag is deployment policy, never a model/reservation argument.
Restart, worker/run ID change, delegation and amendments do not reset root or
mandate spending. Enabled daily counters change at midnight UTC; root/mandate counters
continue. Migration057 delegation contracts add exact immutable child scopes to
every reservation on that branch. Their limits/deadlines restrict existing scopes
further, never replace them. Scheduling acceptance is tracked separately in
`delegation-contract.md`.

Finite counters cover turns, conservative tokens, monetary micros, calls,
children, reserved active milliseconds, tool egress and provider input bytes.
Zero is a hard limit. Absolute deadlines bound elapsed work; each model/tool
context uses the earliest reservation or scope deadline. Native Flash reserves
its declared 1,048,576 context and 8,192 output tokens instead of guessing exact
serialized token counts. Observed usage is separate; missing usage is null.

The trusted budget wrapper records optional complete turn-level event usage for
each newly admitted operation, including a tool proposal or batch. It falls back
to legacy final-only usage and accepts identical final/event/terminal repetitions
without another observation or charge. Conflicting duplicate totals or ambiguous
legacy/event fields fail closed, preserving the first committed measurement;
negative/inconsistent/overflowed or provisional counts are never recorded.
Observed allowance overrun is retained as a measurement and stops the turn before
tool or final release; already streamed deltas are not rewound.
For native Antigravity High, a known complete total additionally bounds generated
tokens as `max(candidate_output, total - prompt_input)` against the admitted
output allowance. High's declared generation ceiling includes thinking. This
does not label generic adapters' cached/other token dimensions as generation,
change the stored visible input/output observations, infer billing, or refund
reservations. A missing total preserves the legacy pair-only check and remains
unavailable for full-token-cost acceptance. Missing/incomplete/cancelled
observations remain NULL.

The existing source guard must release current final/tool proposals before their
usage reaches the wrapper. Denied stale source/task authority cannot admit another
request or turn a pending NULL into successful work. A complete measurement
already recorded before later tool failure/cancellation remains historical data,
not renewed authority; generic `ObserveModel` stays a measurement operation.
Reservations are never refunded or replenished. This forward-only wiring never
backfills earlier attempts whose provider counts were not retained.

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
The [bounded model retry](model-retry-policy.md) spends a fresh reservation and
operation. Admission exposes the effective deadline's trusted origin so a nominal
model-turn timeout can be distinguished from an earlier scope or authority limit.
Earlier attempts and NULL measurements remain immutable across recovery.
The same single allowance can instead recover an owning watchdog lease-observation
deadline after fresh authority/source/claim proofs. The cancelled attempt stays
spent; its original worker deadline caps recovery. A second timeout of either
kind cannot claim a separate retry. See the retry policy for the exact boundary.

Loops normalize root, pinned task/constraints revisions, exact tool/contract and
canonical arguments across restart. Equivalent numbers/object order compare
without floating point loss. Typed error categories ignore messages; success
observations strip only known volatile metadata. The next call after the repeated
observation limit stops with repeated_action_without_progress. Unknown non-read
effects throughout the task tree block new non-read calls even with a different
tool. Independent authorized reads may reconcile. Compensation needs a new
explicit authorized action; no unsupported exactly-once guarantee is claimed.

For a conversation without Task, independent durable `runs.input_event_id`
values are progress boundaries for repetition only. Session/owner identity still
comes from the run. The repetition reader attributes existing observations via
their permanent effect's initial run binding and that run's original Input event;
only a proved different Input in the same owner/session is excluded. A new run
for the same event, answer resume and process restart retain the same repetition
history. Missing or inconsistent provenance keeps the old restrictive meaning.
Observation rows/keys are not migrated, deleted or rewritten. Root-task/child
repetition stays unchanged. Conversation/day and owner/day spending, historical
model attempts and the session-wide UNKNOWN/executing-write guard do not acquire
a new scope or allowance when a new owner Input is accepted.
Core and Broker both embed this reader. A working rollout must update both;
an older process can still reject a new conversation under the former session
scope. Mixed versions do not gain permission or discard existing observations.

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

The optional controlled-browser resource hook reads the already reserved exact
permanent effect with `Controller.ToolDeadlineTx` on the same ClaimGrant
transaction. It validates complete owned scopes and finite external-call/time
allowances, then clamps the resource deadline; it spends nothing twice and does
not refund uncertain execution. Resource conflict/expiry rolls back the entire
ClaimGrant transaction, including its new reservation and grant CAS. No second
pool connection or RPC is used while these locks are held. See
[resource adapter](../../internal/resourceexec/README.md).
