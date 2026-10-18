# Shared schedule clock and task fence

Pure Go daily/weekday/interval/one-shot clock calculation with embedded IANA rules,
deterministic slots and bounded missed-run coalescing. Both initiatives and agent
jobs use it. `CheckTaskTx` adds the owning schedule's cancellation/source fence
to ordinary task authority checks; it cannot authorize a model or tool call.

See [`agent-schedules.md`](../../docs/architecture/agent-schedules.md).
