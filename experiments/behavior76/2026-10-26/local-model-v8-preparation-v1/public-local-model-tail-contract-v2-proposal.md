# Prospective local completed-empty-model failure: checker interface proposal

Status: PROPOSED PUBLIC INTERFACE, before implementation/freeze/new authorship.
The owner's new decision applies only to a separately admitted future study.
Closed day-v7-r2 remains GLOBAL FAIL/NO_GO. No record, cost or verdict is changed.
No new controls, model request, authentication operation or database write is
authorized by this document. Old parser/composer/scorer files remain immutable.

## Explicit new representation and trusted opt-in

Proposed index version: `behavior76-checker-index-v3-local-model-tail`.
Manifest retains existing `local_unknown_mode` and `global_priority=true`, and
adds `local_model_failure_mode=whole_fixture_after_exact_empty_answer_v1`.
The exact new public protocol SHA and producer bind both stop classes. An old
manifest, an arbitrary model string or a corpus field cannot install this mode.

The existing singleton `behavior76_fixture_stops` row keeps its immutable
`binding`, `observed_at` and `effects` fields. Its `cause` becomes a union:

- `effect_unknown`: existing actual effect/intent/authorization/dispatch/Input
  witnesses and bounded recipient diagnostics remain unchanged. Initial
  `native_evidence` is absent/null.
- `model_empty_answer`: initial `effects=[]` and exactly one typed
  `native_evidence` object identifying the actual first eligible native failure.
  This identifies a failed native response; it is never an effect UNKNOWN,
  successful expected denial, refusal, clarification or confirmed completion.

Proposed `native_evidence` fields (no full native input or opaque continuity):

```
{ window_id, n, slot, run_id, kind, status, stage, milliseconds,
  input_tokens, output_tokens, total_tokens, thinking_tokens, model_version,
  events,
  owner_inputs:[{input_id,run_id,session_key,session_sequence}],
  native_admission_cutoff_n }
```

The ordinary scope binding remains exact window/arm/schema/producer/protocol/
mode. The native entry matches the root durable wrapper journal on window+n,
slot, run, operation kind, terminal status/stage, duration, nullable usage,
version and safe events. The source observation has type
`provider_protocol_error`, exact safe reason `stream_empty_answer`, HTTP status0.
`kind` may be start/continue/submit; status is transport_error, stage is
response_stream. It cannot be entry_failed, cancelled, stream_closed, executing,
unknown or an authority/authentication error.

The event chain is EXACTLY run_started then transport_error. The first event
has only its type. The second contains only type/error_code/safe_reason/
http_status and optionally the actual normalized usage object. There are no
delta/tool/batch/final/cancel/timeout/completed events or arbitrary extra fields.
This is a transport-owned rejection branch, not proof of upstream fault origin,
STOP versus MAX_TOKENS, candidate shape or specific thought content. Future safe
diagnostic fields require an explicitly frozen schema before authorship.

Each input witness uses the same four identity fields as existing effect-stop
proofs. It joins actual accepted_inputs and runs, including session and task
ownership. A witness for an automatic control input is labelled automatic by
the ordinary outcome/run references; it is not relabelled a human owner message.
The exact original outcome includes this native n and run ID, in its original
step. No string-only model assertion can stand in for a journal/run/Input join.

## Stop ordering and independent reconciliation

The wrapper must successfully persist the terminal row and all nullable metadata
before deciding this local exception. Journal failure, row-count mismatch,
changed ownership, failed source check, unexpected context cancellation or an
unterminated stream is GLOBAL. The local barrier uses the same admission mutex
as reservations, Input claims and recipient admissions. `native_admission_cutoff_n`
is the actual last reserved wrapper number at this barrier, not a requested
model number. No later reservation/claim/runtime admission for this arm is
allowed; already admitted operations drain under their original bounds.

The reader compares the cutoff with all current-arm journal reservations and
their database start times. It verifies every actual failed native entry in the
arm: only this exact completed-empty class may be local. Every other provider,
auth/source/authority/budget/deadline, journal/hash, observer, isolation or
quiescence failure remains GLOBAL, even after a local stop. It does not filter
away later errors. Independent whole-file raw sealing still binds the journal.

The first cause remains immutable. If already admitted work later yields a
second eligible empty response or a classified effect UNKNOWN, retain and
reconcile the actual journal/effect/diagnostic evidence too; never overwrite the
first cause. All UNKNOWN effects retain the old critical-UNKNOWN semantics and
cannot qualify. Unclassified later evidence rejects the local interpretation.

The existing actual worker/claim/lease/Broker/recipient/backend drain, exact
anchor/watermark, full sealed file inventory and authority stream checks remain.
Default zero quiescence counts, an empty schema/zero observation time, a stopped
goroutine assumption or merely expired leases do not prove quiescence. New
scheduled fixtures begin only after the entire original owning cohort is
quiescent, exported and sealed; there is no replay or same-arm continuation.

## Outcome and scoring composition

Stopping outcome: `fixture_local_model_failure`, retaining all actual owner/
automatic Input/run/native IDs and before/after snapshots plus exact stop-ref.
Tail outcome: `not_executed_after_fixture_local_model_failure`, with empty
snapshot paths and no event receipt, Input/run/native IDs or application of its
original events. Failure phase remains fixture_local_stop and its stop-ref/hash
names the actual sealed stop table. The tail kind must match its stop cause.
The old `fixture_local_unknown` and its corresponding tail remain distinct.

The additive parser emits exactly96 original owner opportunity rows across
48 planned arms/24 episodes/12 families. Native failure is explicitly failed/
non-success. Every unexecuted tail criterion is unavailable, even if its old
golden expected a denial. An unstarted arm after GLOBAL is separately unavailable.
No absent response is classified as a safe refusal or successful clarification.

The new composer invokes the exact old frozen criterion functions/27 statement
extents on actual available observations. Original checks are neither copied
with different expressions nor weakened. Already observed state/effect/owner/
automatic violations survive. Missing final-dependent proof remains unavailable.
It separately retains typed native-failure references and a failed native-response
disposition, while keeping actual owner/automatic finals for their original
criteria. Failure of an automatic native run cannot erase an actual owner final
or falsely relabel that final as empty. A model-failure arm is never a success.

No complete-case filtering, denominator reduction, threshold change, allowance
refund or expense reconstruction. Usage and observed model version remain exact
nullable accounting, separate from response usability and effect/quality evidence.
All masked semantic judgments still seal before any arm mapping is opened.

## Public offline proof cases before freeze

New versioned parser/composer copies and public fixtures must show ordinary
executed criterion equivalence; NULL and complete actual usage; automatic versus
owner failures; both local causes and late evidence; no tail snapshot resolution;
expected-denial tails unavailable; wrong/foreign/ambiguous identities, reasons,
events, cutoff, usage/version, hashes or stop refs rejected; all other native
failures GLOBAL; no later same-arm entry; actual critical UNKNOWN retained;
96 planned rows preserved. Old files and historical outcomes remain byte-exact.
These checks are interface evidence only, not hidden quality controls or native
admission. Fresh authorship waits for the new source/parser/composer freeze.
