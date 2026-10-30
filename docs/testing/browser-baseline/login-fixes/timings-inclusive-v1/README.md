# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.click | effect_unknown | 1 | 1 | 41.213 | 41.213 | 41.213 |
| browser.close | completed | 1 | 1 | 46.398 | 46.398 | 46.398 |
| browser.freeze_snapshot | completed | 1 | 1 | 1.177 | 1.177 | 1.177 |
| browser.open | completed | 1 | 1 | 13.671 | 13.671 | 13.671 |
| browser.prepare_fill | completed | 2 | 2 | 44.523 | 18.968 | 70.078 |
| communication.message | completed | 1 | 1 | 15.193 | 15.193 | 15.193 |
| communication.message | validation_rejected | 1 | 1 | 33.697 | 33.697 | 33.697 |
| credentials.search | completed | 1 | 1 | 10.209 | 10.209 | 10.209 |
| tools.load | completed | 1 | 1 | 22.155 | 22.155 | 22.155 |
| tools.search | completed | 7 | 7 | 14.24 | 9.629 | 28.772 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.
