# Fixed schedule control

Five immutable `schedule.*` contracts and a bounded authenticated private HTTP
client/handler. Arguments carry requested instruction and timing only. Broker
supplies exact private dispatch identity; Orchestrator independently reconstructs
current execution authority. The service token never grants owner authority.

Schema, timing, lifecycle, configuration and validation are specified in
[`agent-schedules.md`](../../docs/architecture/agent-schedules.md).
