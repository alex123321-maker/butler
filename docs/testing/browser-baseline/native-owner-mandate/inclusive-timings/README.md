# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.click | effect_unknown | 2 | 2 | 43.671 | 36.269 | 51.072 |
| browser.close | completed | 1 | 1 | 4.297 | 4.297 | 4.297 |
| browser.freeze_snapshot | completed | 2 | 2 | 0.085 | 0.075 | 0.095 |
| browser.open | completed | 3 | 3 | 14.818 | 12.741 | 17.805 |
| browser.prepare_fill | completed | 6 | 6 | 5.496 | 4.66 | 5.946 |
| browser.prepare_fill | failed | 2 | 2 | 8.586 | 6.83 | 10.342 |
| browser.read | completed | 1 | 1 | 38.393 | 38.393 | 38.393 |
| browser.snapshot | completed | 2 | 2 | 5.648 | 5.497 | 5.799 |
| communication.message | completed | 1 | 1 | 18.284 | 18.284 | 18.284 |
| tools.load | completed | 3 | 3 | 12.264 | 11.897 | 12.699 |
| tools.search | completed | 8 | 8 | 8.84 | 4.916 | 31.614 |
| tools.search | execution_stopped | 1 | 1 | 5.646 | 5.646 | 5.646 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.
