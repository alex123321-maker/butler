# Closed fresh pilot: operational diagnosis and next proof gates

This is source plus execution-metadata analysis of the immutable producer
`9c83730d5858cf3f932fea78ba3e070458d288f5`. It does not disclose holdout
expectations, model answers, quality scores or arm mapping. No native call,
Core replay, original-data mutation or authority/timeout change was performed.

## First unknown effect

The actual admitted `http.request` is a GET. The first GET in the run has a
confirmed effect and recipient receipt. The next call has a real intent,
authorization and dispatch, then `effect_unknown`/dispatch outcome `unknown`.
Its durable evidence is `runtime_response`, method `read_result`, verdict
`effect_unknown`, with no result digest, normalized-result digest or preserved
runtime error category. There is no recipient receipt or object-event row for
that dispatched action. The public ToolCall error is the generic nonretryable
validation error "external effect outcome is unknown; verify before any new
attempt". This is an execution-side uncertain dispatch outcome, not an observed
provider protocol error. The request is GET, and the frozen recipient executes
its CAS condition only for non-read operations; attributing this outcome to CAS
is unsupported and inconsistent with that source path.

The actual effect binding's owning task differs from the addressed object's
durable owning task. The frozen recipient checks that equality after loading
the object, before inserting object events/receipts or committing its
transaction. If execution reaches this check, it returns a Go error
"synthetic resource belongs to another task". A rollback then preserves the
fixture state. This supplies a concrete, independently checkable rejection
precondition. It is a strong source-supported explanation, **not a retained
trace of the actual error return**: neither gRPC status nor runtime rejection
reason was saved in the durable records, console, or raw export. Other
pre-return failures cannot be excluded retrospectively.

The production Router returns a nil result on runtime RPC error. The Broker
passes runtimeError=true to Outcome; Outcome starts from StateUnknown and
confirms only an identity-correlated completed result with supported evidence.
It therefore conservatively records this uncertain outcome. Even an explicit
failed read with useful validation details retains the effect_unknown journal
state under the current production contract. Changing the fixture from a Go
error to a failed ToolResult would improve diagnostics; it would not by itself
prove a confirmed no-effect outcome or remove the global-stop condition.

The missing receipt alone is not a generic proof of absence of an external
effect. In this declared transactional synthetic backend it can be interpreted
alongside the immutable transaction code, complete receipt/event tables and
independent object state. Do not rewrite the original Broker UNKNOWN or call
the tool invocation a passing boundary merely because the safety check is
plausible. The current study's continued admissions after this outcome remain
an immutable protocol deviation.

Before any fresh study, a public zero-native real-Broker regression should
retain: actual method/effect/run/task/object binding; exact request/contract
hashes; recipient entry and phase; bounded enum rejection reason; gRPC status;
SQLSTATE (if present); before/after hashes/versions; actual receipt/event
presence or absence; durable Broker effect and dispatch state. Successful
GET/prepare/POST, stale CAS, foreign-task rejection, lost-reply-after-commit,
ownership expiry and malformed response must be distinguishable. A diagnostic
must never relabel an after-commit lost reply as a proven pre-effect rejection.
No model-generated reason is an authority or no-effect witness.

## Task authority deadlines

The fixture already uses OpenWithAuthorityReserve(MaxConns=10): nine business
connections and one reserved authority connection. Lease, Task, budget and
Catalog observation paths are wired to it. Absence of an authority reserve is
therefore not an established explanation.

The production watchdog creates one shared750ms context, first reads the lease,
then calls Task.Store.ValidateRunAuthority. ValidateRunAuthority begins through
authorityReads, checks the current run/task binding and calls
CheckTaskAuthorityTx. That calls scheduling.CheckTaskTx and
delegation.CheckTaskTx, each through the same transaction, then reads the owner
assignment (and additional mandate/goal checks only if an assignment exists).
The logged phase names the stage reached, not the SQL statement that consumed
the time. Logged elapsed2074/2727ms does not establish query execution time,
pool-wait time or model thinking time. Timer/cancellation delivery can also be
delayed; no causal claim follows from elapsed time alone.

Only the already authorized single-fixture archive was restored into the new
checker diagnostic database. It remains default_transaction_read_only=on.
No opaque provider continuity, full control backup or authentication material
was selected. Read-only predicate/plan observations:

* PostgreSQL17.11, jit=on, pg_jit_available()=false in this diagnostic instance.
* tasks1; scheduled_occurrences0; task_delegations0; task_authority_bindings0.
* Current run/task active/revision predicate matches1.
* EXPLAIN ANALYZE execution: current run/task join0.078ms; scheduling recursive
  predicate0.220ms; delegation recursive predicate0.339ms; assignment lookup
  0.020ms. Plans have low costs (approximately2,15,20,8), no JIT block.
* The exact delegation FOR SHARE query was EXPLAIN-only. Its ANALYZE measurement
  omitted only the lock clause to remain read-only; that observation does not
  measure production locking or the complete PL/pgSQL function.

These measurements do not reproduce the historical failure. Restored physical
statistics, current load and process scheduling need not match the collection.
Historical SQL/JIT/pool contention remains unproved. In this diagnostic instance
JIT is unavailable, so JIT is not an explanation of these measured predicates.
The reserve is shared by several bounded readers; reserve existence alone does
not prove every reader always obtains it inside a shared750ms watchdog budget.

The next useful public scripted gate should record value-free acquire start/end,
remaining deadline after lease read, transaction begin/end, query family/hash,
query elapsed, SQLSTATE, cancellation cause and safe pool counts. Include actual
lease+task+source+budget observations, both ordinary concurrent input sessions,
typed lifecycle/restart, and an intentionally occupied reserve. Keep the
production bounds, authority denial and shared deadline unchanged. A phase
failure must seal the collector globally before any later admission. No larger
timeout, JIT toggle or extra capacity should be justified by this diagnosis
without a separate reproduced cause and prospective review.

Evidence: `fresh-authority-plan-metadata-v1.sql/.sh/.log`, last-slot metadata
receipts, immutable raw tool/effect/receipt rows, and the pinned public source.
