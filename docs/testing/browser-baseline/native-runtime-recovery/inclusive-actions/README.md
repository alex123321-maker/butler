# Inclusive action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.

Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.

| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.click | effect_unknown | 4 | 4 | 37.699 | 36.039 | 52.646 |
| browser.close | completed | 3 | 3 | 11.54 | 11.218 | 12.353 |
| browser.freeze_snapshot | completed | 4 | 4 | 4.914 | 0.375 | 5.577 |
| browser.navigate | effect_unknown | 1 | 1 | 91.13 | 91.13 | 91.13 |
| browser.navigate | failed | 1 | 1 | 34.217 | 34.217 | 34.217 |
| browser.open | completed | 11 | 11 | 16.586 | 6.133 | 38.655 |
| browser.prepare_fill | completed | 8 | 8 | 19.044 | 7.501 | 51.231 |
| browser.prepare_fill | execution_stopped | 1 | 1 | 469.174 | 469.174 | 469.174 |
| browser.prepare_fill | failed | 8 | 8 | 44.058 | 18.005 | 69.209 |
| browser.read | completed | 1 | 1 | 36.317 | 36.317 | 36.317 |
| browser.snapshot | completed | 4 | 4 | 19.13 | 5.166 | 22.741 |
| communication.message | completed | 2 | 2 | 18.92 | 13.455 | 24.384 |
| communication.message | validation_rejected | 4 | 4 | 20.864 | 16.051 | 40.14 |
| tools.load | completed | 8 | 8 | 17.816 | 5.466 | 25.652 |
| tools.search | completed | 23 | 23 | 14.892 | 5.04 | 152.755 |
| tools.search | effect_unknown | 1 | 1 | 115.315 | 115.315 | 115.315 |
| web.fetch | completed | 4 | 4 | 14.816 | 6.811 | 23.272 |

actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.
