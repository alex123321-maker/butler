# Telegram owner channel

## Explicit publication composition

`NewExplicitAdapter` provides the new style through the same Telegram adapter:
durable `telegramrich.Poller`, existing Communication outbox worker and native
message/photo/Rich Message sender. Construction requires matching bot/binding,
the owning Store/source/export guards and existing Task Controller. Runtime
verifies the actual bot before polling/sending. The constructor replaces the
sender's authorization hook with the owning `DeliveryStore.AuthorizeSend`.
The explicit worker is bound to its immutable destination snapshot. It never
claims or expires another bot's or Web channel's publication rows, including
bots which share the same owner's numeric chat ID.

Model finals/deltas/tool/status events cannot create messages in this mode.
Only stored publications send; legacy notifications are refused. Unsupported
approval/widget UX fails closed. The legacy constructor refuses publication rows
instead of using its weaker text-only sender. Its ordinary result path remains.

Default Bootstrap does not use the new constructor. No environment flag or
production switch is added. New-conversation/model admission belongs to #130;
Web belongs to #129. Native test-bot evidence and code boundaries are recorded in
[the scoped acceptance](../../../../../docs/ai/communication-telegram-rich-progress-20261021.md).

## Legacy owner controls

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
