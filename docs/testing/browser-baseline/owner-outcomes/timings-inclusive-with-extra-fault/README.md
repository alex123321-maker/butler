# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.click | effect_unknown | 3 | 3 | 88.362 | 37.718 | 222.399 |
| browser.close | completed | 3 | 3 | 113.18 | 73.615 | 200.199 |
| browser.extract_text | completed | 1 | 1 | 13.357 | 13.357 | 13.357 |
| browser.freeze_snapshot | completed | 3 | 3 | 0.617 | 0.129 | 5.262 |
| browser.open | completed | 8 | 8 | 8.868 | 5.88 | 14.17 |
| browser.prepare_fill | completed | 6 | 6 | 16.325 | 6.34 | 109.976 |
| browser.prepare_fill | effect_unknown | 1 | 1 | 94.78 | 94.78 | 94.78 |
| browser.prepare_fill | execution_stopped | 1 | 1 | 53.394 | 53.394 | 53.394 |
| browser.prepare_fill | failed | 3 | 3 | 28.873 | 12.687 | 115.958 |
| browser.snapshot | completed | 3 | 3 | 13.633 | 6.57 | 80.521 |
| communication.message | completed | 2 | 2 | 12.412 | 6.942 | 17.882 |
| communication.message | effect_unknown | 1 | 1 | 28.013 | 28.013 | 28.013 |
| communication.message | validation_rejected | 3 | 3 | 17.986 | 10.774 | 33.007 |
| credentials.request_access | execution_stopped | 1 | 1 | 11.603 | 11.603 | 11.603 |
| credentials.search | completed | 7 | 7 | 6.183 | 5.427 | 18.035 |
| tools.load | completed | 10 | 10 | 12.919 | 10.808 | 24.381 |
| tools.search | completed | 38 | 38 | 9.048 | 4.863 | 34.954 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.

This export contains the nine form/login/consent matrix trials plus post-loss-01, one additional fault injection. The extra trial does not replace a failed matrix slot. Use timings-inclusive-original-final for the nine original matrix trials only.
