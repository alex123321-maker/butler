# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.close | completed | 9 | 9 | 4.688 | 3.731 | 6.82 |
| browser.fill | effect_unknown | 1 | 1 | 78.26 | 78.26 | 78.26 |
| browser.fill | execution_stopped | 1 | 1 | 179.142 | 179.142 | 179.142 |
| browser.follow_link | completed | 14 | 14 | 5.903 | 5.121 | 11.48 |
| browser.open | completed | 17 | 17 | 4.97 | 3.502 | 7.06 |
| browser.snapshot | completed | 12 | 12 | 4.989 | 3.688 | 10.947 |
| browser.submit_get_form | completed | 9 | 9 | 8.218 | 5.458 | 17.188 |
| browser.submit_get_form | effect_unknown | 2 | 2 | 7.47 | 7.258 | 7.683 |
| browser.submit_get_form | failed | 4 | 4 | 7.397 | 6.409 | 9.964 |
| communication.message | completed | 12 | 12 | 12.738 | 5.443 | 31.085 |
| communication.message | validation_rejected | 3 | 3 | 10.221 | 10.045 | 14.97 |
| http.download | completed | 1 | 1 | 4.611 | 4.611 | 4.611 |
| tools.load | completed | 23 | 23 | 9.045 | 3.467 | 12.676 |
| tools.search | completed | 66 | 66 | 4.802 | 2.947 | 32.47 |
| web.fetch | completed | 14 | 14 | 6.432 | 4.016 | 28.646 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.
