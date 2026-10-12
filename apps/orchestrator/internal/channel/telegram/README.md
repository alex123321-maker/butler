# Telegram owner channel

The adapter binds controls to allowed chat, numeric sender ID, session and stable
Telegram update delivery. Owner commands bypass provider login and model execution.
`/task TASK_ID` reads safe recorded state; `/amend TASK_ID INSTRUCTION`, `/pause
TASK_ID`, `/resume TASK_ID`, `/stop TASK_ID` use the existing durable controller.
`/mandate_revoke MANDATE_ID CURRENT_REVISION` revokes an exact stored authority.
No text/model owner ID or raw capability is accepted.

Short observations keep Task, attempt answer, verified result, effect and channel
delivery distinct. Only bounded safe references/states are sent; no source bodies,
provider errors, raw arguments, credentials or hidden reasoning are included.
Approval prompts use service-derived finite consequences/version metadata.

Tests use a local fake Telegram HTTP server only. Run `scripts/dev/owner-state-lab.sh`
on laba for channel tests and owning PG/HTTP regressions; no human messages or
model calls are part of this acceptance.
