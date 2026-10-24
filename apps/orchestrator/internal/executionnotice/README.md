# Execution notices

Kernel-owned, fixed Russian lifecycle notifications for the explicit owner
communication profile (migration097). PostgreSQL Input/approval/publication
facts determine queued, working, waiting, blocked, failed and missing-response
states. No model call, source text, raw error or secret is used. These records
do not complete a task, publish a model final or authorize an action.

Each verified binding/version has a durable activation watermark. Startup and
restart reconcile only admitted inputs accepted after that activation; a new
binding cannot adopt old history. Web exposes the latest state per input as
available, never read. Telegram uses the existing channel-only outbox Worker
against this module's separately owned receipts, with current binding, owner,
state and finite claim checks. Only actual numeric channel ACK means sent.
Lost/malformed ACK and expired sending claims remain UNKNOWN without resend;
only confirmed absence can retry with bounded backoff. Superseded states are
not sent. Successful published answers do not generate another Telegram result.

The optional owner Web endpoint is `/api/v3/communications/states`. Notices have
no public write endpoint or model schema. Delivery is independent of Input
worker cancellation, inference allowance and publication source access, so a
safe fixed failure notice can explain a denied or failed model attempt.
