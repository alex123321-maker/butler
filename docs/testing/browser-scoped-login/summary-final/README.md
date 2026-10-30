# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.login | completed | 1 | 1 | 132.958 | 132.958 | 132.958 |
| browser.login | effect_unknown | 1 | 1 | 23.178 | 23.178 | 23.178 |
| browser.open | completed | 6 | 6 | 9.532 | 5.318 | 12.94 |
| browser.snapshot | completed | 4 | 4 | 38.41 | 7.219 | 124.447 |
| communication.message | completed | 1 | 1 | 24.029 | 24.029 | 24.029 |
| credentials.request_login | execution_stopped | 1 | 1 | 22.126 | 22.126 | 22.126 |
| credentials.search | completed | 9 | 9 | 9.591 | 5.838 | 22.6 |
| tools.load | completed | 7 | 7 | 11.402 | 8.535 | 17.352 |
| tools.search | completed | 23 | 23 | 9.899 | 4.165 | 26.151 |
| tools.search | execution_stopped | 1 | 1 | 4.623 | 4.623 | 4.623 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.
