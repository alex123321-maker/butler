# Antigravity native transport

The explicit `gemini-3.8-flash-high` profile uses the measured Desktop Cloud Code
contract through native Go: daily `/v1internal:streamGenerateContent?alt=sse`,
model/project/request wrapper, per-turn request identity, Butler-owned run session
and thinkingBudget=-1. Output remains8192 to match durable admission. Manager
discovers the project with the same refreshed OAuth and requires confirmed paid
Pro/Ultra; an imported legacy project never selects a route. Wrapped
`response.candidates`/usage becomes the existing Butler event contract. No endpoint
or model fallback is performed. Select it with `BUTLER_ANTIGRAVITY_MODEL` and the
matching `subscription_no_overage` execution budget entry; main deployment is
unchanged by this patch. See [High acceptance](../../../docs/ai/antigravity-high-acceptance-20261012.md).

Only supplied Butler inputs/system instructions and scoped declarations are
serialized. No Desktop reminders, third-party identity prompt, system prefix,
prompt rewriting or additional tools are added. Fixtures verify exact system/user
text, native calls/signatures/continuation, wrapped SSE and failure-before-write.

Live admission receipts retain Core's `{status,result}` envelope inside the
correlated native `functionResponse`. The child's ID stays at
`response.result.task_id`, including when the receipt state is waiting and
`input_id` is empty. A three-request synthetic High fixture checks both sequential
receipt identities, original native call IDs/signatures and a separate terminal
JSON answer after tool-phase commentary. This is zero-inference serialization
evidence, not a successful model experiment or an explanation of an old model final.
An isolated actual High delegation window additionally completed two Broker
admissions and two correct child extractions after process loss. It stopped at
the parent's exact receipt-projection failure before width2/later controls;
[safe evidence](../../../docs/ai/delegation-high-acceptance-20261012.md) records5
new calls without retry. Full delegation acceptance and deployment remain pending.

The following AICode details and earlier investigations describe the retained
default tiered profile and their historical evidence.

The retained tiered profile uses production AICode v1 SSE with the requested experience
`gemini-3.8-flash-tiered`. The flat request uses `aicode.experience`, ordered
contents, optional system instructions/Butler function declarations and an
8192-token output allowance. It omits the mutually exclusive `model`, Cloud Code
project/request wrappers and optional CLI session/entitlement fields. Flat SSE
responses provide candidates and usage metadata.

The installed CLI 1.3.3 was captured offline against synthetic HTTPS with fake
credentials and Docker networking disabled. Its default fixture path used Cloud
Code; explicit `USE_AICODE=true` emitted an AICode **v1** request with `sessionId`.
Thus neither that field nor the protobuf namespace identifies a v1main-only HTTP
contract. Earlier production probes rejected additional session/entitlement
fields; the capture does not prove their acceptance for Butler. Runtime behavior
is unchanged. See [the diagnosis](../../../docs/ai/antigravity-quota-diagnosis.md)
and [the isolated capture command](../../../scripts/dev/antigravity-wire/README.md).
Account subscription membership is confirmed; per-request subscription metering
is still unknown.

The separate Desktop2.22 pre-dispatch capture used the installed Windows frontend
with an official same-CL Linux backend on isolated laba. It measured Flash-high,
daily Cloud Code v1internal/SSE, a65536 allowance and15 declarations with a much
larger client context. All primary/auxiliary writes were denied before Google;
actual Go completed synthetic SSE with its unchanged8192 default. This is not
the original Windows backend/owner-dialogue capture, a causal transport defect,
or permission to copy Desktop fields/routes into this adapter. See
[safe evidence](../../../deploy/antigravity/desktop-interception-20261012.json).
No new real attempt or production behavior change resulted.

Subsequent owner-authorized live checks completed both actual CLI (Flash-medium,
daily CloudCode) and this Go Provider (Flash-tiered, AICode) with a diagnostic512
cap. A separate Go request preserved the original body and8192, receiving
RESOURCE_EXHAUSTED/HIGH_DEMAND; native `model_capacity_unavailable` was correct.
The eight-minute gap leaves cap sensitivity unproven. Production behavior is
unchanged. These are bounded text checks, not full delegation/tool acceptance.
All21 agent-tracked attempts and seven immutable reports are retained in the
diagnosis. The private comparison helper is not a production transport fallback.

Only the typed providerauth source supplies token/expiry and, for High, private
discovered routing metadata through `AntigravityCloudCodeTokenSource`. Transport
does not read credentials or run the CLI. Native calls must match supplied aliases
and schemas; Tool Broker remains the sole executor. Exact function parts and opaque
thought signatures remain private and run-local. Durable recovery starts fresh
with Butler observations as data, while live continuation correlates the original
native call and function response. Cancellation closes HTTP and discards context.

HTTP 429 with the bounded, recognized `HIGH_DEMAND` reason uses the fixed
`model_capacity_unavailable` subtype and reports temporary model
capacity loss. Other 429 responses remain generic; neither proves exhausted
subscription quota. Raw Google diagnostics stay private and no implicit retry runs.

See [the operator guide](../../../deploy/antigravity/README.md) for private import,
the unsupported third-party integration policy, proxy configuration and real
validation status. Fixtures verify wire shape, safe failure handling, signatures,
schema guards and cancellation; they do not prove Google model access.

`InferenceLimits` declares 1,048,576 input plus 8,192 output tokens for conservative
kernel admission. Private `transport.WithInputByteLimit` bounds the exact complete
serialized request, including private retained history, schemas and tool results,
before HTTP write; unset callers use a fixed 8MiB cap. Operator subscription cost
basis is separately required by production execution budgets. Neither this quote
nor model availability proves a successful authenticated request.
