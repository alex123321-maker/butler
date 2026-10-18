# Scheduled jobs

Owns PostgreSQL scheduled jobs, exact control receipts, occurrence history and
reminder notifications. Clock/DST arithmetic comes from `internal/scheduling`;
normal Task/Input/Core and Tool Broker own execution. Reminders reuse the channel
delivery worker without creating a model run. Both creation and every later
execution/delivery retain current owner/source/authority guards.

See [the implemented contract](../../../../docs/architecture/agent-schedules.md).
Run `pwsh -NoProfile -File scripts/dev/schedules-local.ps1` from the repository
root for actual isolated PostgreSQL/Broker/Input/Core, race and compatibility tests.
