# Initiatives

Actual Broker/executor prepared-patch acceptance and its remaining integration
limits are recorded in `docs/architecture/initiative-acceptance.md`. The fixture
uses owner verification, not a model's final text, to complete the admitted task.

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

After shared budget waits, promotion rechecks source active hours and observation
freshness against the current PostgreSQL clock. A UTC-day rollover defers with
`admission_day_changed` so the next pass checks the new day's model capacity and
attention allowance. These deferrals create no task, receipt or model charge.

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
The real-process prepared-correction fixture now executes the promoted task
through the ordinary durable Input worker and Core with an injected provider.
It checks the recipient's actual stored patch and Broker check, then uses an
independent owner Task Service verification. The model final stays unverified.
A separate digest worker uses the production Telegram adapter against a synthetic
HTTP Bot endpoint, retains its actual ACK and sends once. This component fixture
uses four scripted provider turns; live High generative acceptance remains a
separately bounded gate for issue #15.

`digest_channel` is optional and must match the stored session channel. A separate
durable outbox coalesces only admitted/verified-result changes into count-only
notices, at most one per six-hour window and four per owner UTC day. Active hours
and current source/goal/mandate are rechecked before Telegram delivery. Explicit
absence may retry within six attempts/24 hours; uncertain acceptance is retained
as `delivery_unknown` and never automatically sent twice. Web `result_ready`
means a stored notice is available in the owner view, not that it was read.
Migration056 refuses rollback after any notification history.

`broker_project` reads already-saved authorized GET/read observations, pinned by
`check_manifest_hash`, `check_tool_name`, `check_resource` and project revision.
The live source guard is mandatory before/after reading and inside promotion,
including its final check after budget waits. One run is one independent attempt;
the response's claimed origin is ignored. Confirmed historical Broker dispatch
and intent/argument hashes are required. It performs no external request itself.
Configure it in `/initiatives` using a reviewed connector operation; templates
and schedules remain owner administration, outside the model tool catalog.
Bootstrap wires the source-lifecycle guards. The repeated-check-to-verified-
prepared-correction E2E derives observations from three actual exactly approved
Broker check tasks. Its promoted task runs through ordinary Core/Broker under
the current owner goal/mandate, identical finite Core/Broker budget policy and
current Catalog pins. The optional provider factory receives only the promoted
task ID. It grants no source access, action permission or verification authority.
Native callers must retain their separate attempt window.
The owning-route zero-inference preflight selects explicit High but retains a
scripted transport, checks the native SQL binding at all four ordinary entries
and leaves the separate native attempt counter unchanged.

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

For the zero-inference real-process fixture, sync an isolated lab root with
`scripts/dev/lab.ps1 -Action Sync -RemoteRoot <agent-root>` and run
`bash <agent-root>/current/scripts/dev/initiative-lab.sh <agent-root> broad` on
the lab. The runner creates disposable PostgreSQL, builds the actual Broker and
extension runtime with race detection, runs the app fixture and initiative suite,
checks vet and rejects skips. It mounts neither operator credentials nor native
authentication. See `docs/architecture/initiative-acceptance.md` for all nine
issue criteria and the separate generative gate.
