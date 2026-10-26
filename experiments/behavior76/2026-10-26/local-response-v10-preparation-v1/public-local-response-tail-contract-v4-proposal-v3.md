# Prospective response-format isolation, v10 / checker v4

Draft only: no source/scorer freeze, hidden authorship, model call or native admission follows from this document. All ten closed windows remain immutable NO_GO/incomplete history:723 admissions,713 measured,5,626,388 known full tokens and10 NULL usage observations. The383 v9 and1253 v8 raw files retain their exact original seals. No old response, golden or mapping is reused or reassessed.

The new trusted binding is `fixture-local-response-v10`; index `behavior76-checker-index-v5-local-response-tail`; manifest adds `local_response_failure_mode: whole_fixture_after_exact_response_rejection_v1`. Existing exact empty-answer, exact confirmed HTTP-no-response and classified synthetic effect-UNKNOWN modes remain unchanged. Only trusted constructor code opts in; a model, episode, environment supplied by a model or document cannot select policy.

## Narrow current-source allowlist

For the currently reviewed Antigravity adapter, the new cause `model_response_rejected` can cover only these fixed transport-owned safe reasons:

`stream_json_invalid`, `stream_usage_ambiguous`, `stream_usage_invalid`, `stream_usage_conflict`, `stream_model_version_invalid`, `stream_candidate_unsupported`, `stream_candidate_unexpected`, `stream_finish_rejected`, `stream_part_invalid`, `stream_part_unsupported`, `stream_thought_invalid`, `stream_function_ambiguous`, `stream_content_missing`.

These are local parser rejection categories, not proof that upstream, the adapter or a particular safety/finish mechanism caused the fault. They remain failed provider outcomes. A malformed provider usage field is never a measured amount or permission to waive the actual budget controller.

Conservative exclusions are required by actual source, not by desired scores:

- `stream_terminal_missing` currently conflates missing terminal/framing with `scanner.Err()` / network body-read failure and the16MiB total limit. Native event-channel EOF cannot disambiguate these wire causes. Keep GLOBAL unless a separately reviewed fixed diagnostic splits them.
- `stream_envelope_invalid` currently also represents non-null `response.Error`, including a possible HTTP200 SSE auth/server error envelope. It is not a pure shape class; keep GLOBAL unless separately split.
- `stream_function_truncated` is a MAX_TOKENS/function limit path which returns the actual `Usage` and `ModelVersion` after complete EOF. The provider forwards those to its error event. Keep GLOBAL as a function/output-limit class; do not erase known metadata to make it eligible.
- `stream_frame_limit` and `stream_automatic_function_execution` remain GLOBAL as limit and authority/effect faults. HTTP/status/auth, cancellation/timeouts, actual budget/source/authority/storage/journal/observer/isolation/quiescence failures and unknown reasons also remain GLOBAL.
- `stream_empty_answer` keeps its prior exact two-event union, including genuinely observed nullable/measured usage/version. It is tested FIRST as model_empty_answer in the new mode and is not intercepted by model_response_rejected or broadened to partial streams.

Any future split/new reason needs public source+checker review and freeze before fresh authorship; no current or previous result is reclassified.

## Exact typed proof

Require `provider_protocol_error`, `response_stream`, `transport_error` status, integer HTTP0 and exactly:

1. `{type:run_started}`;
2. zero or more `{type:assistant_delta,text_delta:{DeltaType:text,Content:<nonempty string>,SequenceNo:<positive integer>}}`;
3. one final `{type:transport_error,error_code:provider_protocol_error,safe_reason:<allowlisted literal>,http_status:0,usage?:<strict actual normalized complete Usage>}`.

All names/types are exact and case-sensitive, no extra/aliased fields. Content is opaque evidence: never execute it, grade it as a final, include it in a diagnostic summary or infer tools/effects from it. Actual `SequenceNo` is the same invocation-turn number across that invocation's deltas; it is not a per-delta increasing counter. It must be1..4 under the existing four-entry/run bound. Aggregate UTF8 text bytes may not exceed the existing16MiB response cap; nonempty deltas therefore also give a finite count at most16MiB. This is a derived existing source bound, not an enlarged model/authority budget.

Only the last error event may have an optional actual normalized complete usage object. Its exact fields are InputTokens, OutputTokens, TotalTokens and optional ThinkingTokens, all case-sensitive non-NULL nonnegative signed64 Go/PG integers with consistent totals/thinking. Explicit usage:null, partial counts, aliases, extra fields, negative/inconsistent/undurable data remain GLOBAL. Every observed count (including zero) must exactly match its durable nullable row column; absence of an observed usage object requires all four row usage columns NULL. Missing optional ThinkingTokens stays NULL, not zero. Model version is independently nullable or an actual valid nonempty gemini-* identifier, preserved exactly in the journal/proof; an observed empty-version pointer cannot be represented faithfully by current NULLIF and must remain GLOBAL. Never erase/coerce known usage/version to make a failure eligible. For stream_usage_ambiguous, stream_usage_invalid and stream_usage_conflict specifically, no usage object and four NULL usage columns are required: a scalar proof cannot resolve the problematic dimension. If known usage is actually present, retain it but keep that outcome GLOBAL, never erase it. Similarly stream_model_version_invalid requires model_version NULL; a non-NULL observed version with that reason remains GLOBAL. Independent valid dimensions remain retained: a valid version with a usage-format error or valid usage with a version-format error may qualify if all other proof conditions hold. No usage property is allowed on start or delta events. Existing parser early-error paths can discard partial previously parsed metadata; journal NULL does not prove upstream absence or zero expense. No historical cost reconstruction is allowed.

No tool/function proposal, batch, final, thought, usage event, second error, late event, alternate terminal or unsupported provider-session event is permitted. Retain all actual safe events after a suspected rejection; any contradictory late event becomes GLOBAL and is not discarded. A raw frame/body/error/credential never enters this typed proof.

Before local stop require actual underlying native-channel EOF, a live unchanged owning context, current run/task/constraints/lease/source authority, exact durable journal write/readback, immutable window/producer/protocol/schema/arm binding, unique actual owner/automatic input witness and the same serialized admission cutoff/root barrier. Export validation cannot reconstruct an unrecorded live check: those remain mandatory source-side gates. First cause remains immutable and all later already-admitted errors/effects are reconciled.

The existing `native_evidence` shape is retained byte-for-byte, with the actual delta objects included in its exact canonical equality to the pinned root journal. New stop cause is `model_response_rejected`, effects `[]`. Effect UNKNOWN still has `native_evidence:null` and its own exact evidence. The whole original isolated arm/task/cohort stops; original events/claims/admissions are not reset or replayed. Drain already-admitted work under original limits, verify true quiescence, stream/inventory seals and global-priority state before considering only the next scheduled arm.

Stopping outcome: `fixture_local_response_failure`. Tail: `not_executed_after_fixture_local_response_failure`, empty snapshot paths and no Input/run/native/event receipt. A skipped expected-denial test remains unavailable. The failed native outcome is FAILED/never success. A failed automatic continuation cannot erase a completed owner final. Partial text is retained as count/evidence reference with unassessed partial quality, not a usable owner final or task/effect completion.

## Additive checker and unchanged decision rules

New files are `local-model-proof-reader-v4.ps1`, `local-tail-adapter-v4.ps1`, `local-scoring-composition-v4.ps1`, `authority-stream-scoring-v2d.ps1`. Old v3 files and all closed seals remain byte-exact. Authority stream v2a validation and all27 original criterion AST extents/functions remain unchanged. Retain24 episodes /12 families /48 arms /96 original owner opportunities; do not change thresholds, candidate, stop-tree authority,4h/UTC-day-fit,768 entries,8/slot,4/run,120s model,20min fixture,750ms authority or9+1 pools.

Public synthetic offline negatives must reject every excluded/unknown reason, wrong stage/type/status/HTTP, extra or nontext delta, inconsistent/invalid sequence, fabricated/inconsistent/aliased metadata or loss of actual known metadata, foreign/ambiguous journal or owner identity, cause/tail mismatch, post-cutoff admission, unsealed/global evidence and lost partial/cost/effect data. Ordinary successful outputs must still match the exact frozen criterion engine. Critical UNKNOWN never passes; local failures and missing tails do not reduce the denominator or qualify promotion/issue closure.
