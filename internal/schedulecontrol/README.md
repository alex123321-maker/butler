# Fixed schedule control

Five immutable `schedule.*` contracts and a bounded authenticated private HTTP
client/handler. Arguments carry requested instruction and timing only. Broker
supplies exact private dispatch identity; Orchestrator independently reconstructs
current execution authority. The service token never grants owner authority.

Revision 2 adds optional `job.monitor=true` for agent tasks that check fresh
evidence and publish only meaningful observation changes. It is invalid for a
static reminder. Old specifications omit monitor and keep their policy.

Schema, timing, lifecycle, configuration and validation are specified in
[`agent-schedules.md`](../../docs/architecture/agent-schedules.md).
