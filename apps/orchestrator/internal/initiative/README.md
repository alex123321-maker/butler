# Initiatives

Go control-plane subsystem for durable observations, daily/interval schedules,
candidate coalescing and ordinary task admission. PostgreSQL owns sources,
source revisions, tick leases, normalized event identities, candidates, audit
decisions and the daily attention counter. Redis owns none of this state.

Owner-authenticated sources define the project reference, task template,
criteria, exact goal/mandate, timezone, active hours, importance/confidence,
distinct failure threshold, cooldown and initial normal/prepare-only policy.
Events contain typed kinds and SHA-256 identities; they cannot supply instructions,
tools, destinations, credentials, models or permission. Delivery replay is
idempotent; changing a delivery's payload conflicts. Re-delivery of one failure
under another delivery ID does not count as another independent failure.

Promotion locks session, source and candidate before checking current owner
authority and shared execution budget. It creates a task and ordered receipt
atomically through their owning services. No model turn is spent until actual
inference. Source cooldown and a durable cap of 20 admitted initiative tasks per
owner UTC day bound interruption. Empty checks and proposals have no delivery
outbox message. The owner pulls a bounded audit and digest delivery states at
`/api/v3/initiatives`.

Sources and schedules are configured at `/api/v3/initiatives/sources` and
`/api/v3/initiatives/schedules`. The initial observation adapter at
`/api/v3/initiatives/observations` requires the owner bearer and unique
`X-Butler-Delivery-ID`; raw logs/URLs are rejected by the typed contract. No model
tool can call these control-plane APIs with that bearer.

Application maintenance advances schedules and consumes leased ticks. An
installed `Observer` may report a bounded typed condition; the default heartbeat
is an empty check and performs no external request. `observer_kind=task_project`
instead reads current owner Task/Run state for the configured `project_ref` and
SHA-256 `project_revision`. Owner task creation may bind the same typed pair.
Distinct failures count once, initiative origin is suppressed, and a newer
verified project result clears the condition. Raw run errors are never selected.
External source adapters
must use Broker-mediated, fixed-target observation rather than arbitrary URLs.
That installed adapter/project-fix execution acceptance is still outstanding:
issue #15 remains active until the prepared correction scenario is verified.

`digest_channel` is optional and must match the stored session channel. A separate
durable outbox coalesces only admitted/verified-result changes into count-only
notices, at most one per six-hour window and four per owner UTC day. Active hours
and current source/goal/mandate are rechecked before Telegram delivery. Explicit
absence may retry within six attempts/24 hours; uncertain acceptance is retained
as `delivery_unknown` and never automatically sent twice. Web `result_ready`
means a stored notice is available in the owner view, not that it was read.
Migration056 refuses rollback after any notification history.

Daily schedules skip nonexistent local minutes, choose the first repeated minute
and deduplicate on calendar date. Interval schedules retain their phase. Downtime
coalesces to one latest tick, or skips a missed window; pending leased checks
prevent historical backlog. The daily missed count records elapsed calendar days
(including a DST-skipped day), rather than asserting actual executions.

Run `Integration` and `Race` on the lab with isolated
`BUTLER_TEST_INITIATIVE_POSTGRES_URL` ending in `_test`. Tests use private schemas
and a two-connection pool: concurrent promotion, duplicate delivery/evidence,
obsolete/healthy conditions, revoked authority, missing/exhausted budget, downtime
and stale tick claims. Migration 040 refuses rollback after any policy/history.
