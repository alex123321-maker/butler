# Browser action timings from existing records

No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.

Tool durations are recorded Core spans including Broker/runtime overhead. Gaps include model, scheduling and context work. Model reservation-to-usage spans are accounting observations, not provider-only latency. State/model/tool spans overlap and cannot be added as disjoint wall-time components. Telegram sent is a persisted API acknowledgement, not a human-read receipt. Failed/unknown tool spans are grouped separately from completed operations.

| Tool | Status | Calls | Measured | Median, s | Min, s | Max, s |
| --- | --- | --- | --- | --- | --- | --- |
| browser.close | completed | 9 | 9 | 0.116 | 0.113 | 0.125 |
| browser.fill | effect_unknown | 1 | 1 | 0.2 | 0.2 | 0.2 |
| browser.fill | execution_stopped | 1 | 1 | 0.113 | 0.113 | 0.113 |
| browser.follow_link | completed | 14 | 14 | 0.337 | 0.323 | 1.382 |
| browser.open | completed | 17 | 17 | 0.35 | 0.33 | 1.824 |
| browser.snapshot | completed | 12 | 12 | 0.118 | 0.11 | 0.133 |
| browser.submit_get_form | completed | 9 | 9 | 0.354 | 0.332 | 0.655 |
| browser.submit_get_form | effect_unknown | 2 | 2 | 0.108 | 0.107 | 0.109 |
| browser.submit_get_form | failed | 4 | 4 | 0.111 | 0.107 | 0.114 |
| communication.message | completed | 12 | 12 | 0.593 | 0.58 | 0.644 |
| communication.message | validation_rejected | 3 | 3 | 0 | 0 | 0 |
| http.download | completed | 1 | 1 | 0.418 | 0.418 | 0.418 |
| tools.load | completed | 23 | 23 | 0.114 | 0.107 | 0.139 |
| tools.search | completed | 66 | 66 | 0.115 | 0.107 | 0.2 |
| web.fetch | completed | 14 | 14 | 0.291 | 0.274 | 0.909 |

actions.csv contains every measured step and gap; trials.csv retains failures and unknown usage. model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates are descriptive; they are not an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps.

Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see observation audits).
