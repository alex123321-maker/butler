# Execution budget contract

Issue #13 implementation begins with `internal/budget` and migration 037.
The ledger foundation is implemented; production enforcement and loop/verdict
integration remain pending and must not be represented as complete.

Budget Service owns scope policies, spent reservations and operation identity.
Task Service supplies immutable parent ancestry and owner identity. Session and
Run Services retain execution authority. Orchestrator requests a conservative
model quote before each new provider turn. Tool Broker reserves external-call
allowance in the same transaction as a permanent effect claim. Delegation must
reserve child allowance before child admission. A model cannot provide scopes,
quotes, reservation keys, monetary rates or authority.

Applicable scopes are the task root, inherited mandate, owner UTC calendar day
and any more restrictive child scope. Conversation uses a separate scope without
requiring a planner or critic. A restart, run ID change or task amendment does not
reset task/mandate counters. Daily scopes change at the declared UTC boundary;
task counters continue to constrain work across that boundary.

Finite limits cover turns, tokens, monetary micros, external calls, children and
active milliseconds; absolute deadlines also bound elapsed work. Zero is a hard
limit. Unknown cost fails admission rather than silently reporting zero. Model
availability, incremental cost, subscription state and API billing permission
are separate facts. No stronger-model fallback is authorized by this ledger.

All scope counters advance atomically under sorted PostgreSQL row locks. A
stable trusted operation ID pins the exact sorted scope set and charge. Reusing
that identity with another quote conflicts; replaying the same admitted operation
does not spend twice. Idempotent reservation is not permission to replay a tool:
the effect journal separately decides whether a dispatch may enter its runtime.

An owned transaction hook uses a savepoint on the caller's connection. Admission
failure leaves neither partial scope spending nor a reservation placeholder.
Domain lock order remains ownership/session/run/task before budget scopes.
Conservative reservations remain charged after crashes and unknown outcomes.
They are reported as reserved allowance, separately from observed actual usage.

Pending enforcement must persist a structured budget stop and the current action
or model continuation, provide a concrete exhausted dimension, cancel live
transport, and avoid treating the stop as verified task completion. Normalized
loop/effect evidence and schema-validated completion remain separate from a
model's final text.
