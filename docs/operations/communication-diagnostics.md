# Communication diagnostics and compatible rollback

The explicit owner composition installs the authenticated read-only
`GET /api/v3/communications/diagnostics` endpoint and adds the same observation
to owner Doctor reports. Normal legacy startup reports `not_composed`. The Web
conversation exposes **Состояние общения** on demand; failure to observe clears
the old snapshot and displays an unknown result. Telegram failed/unknown parts
are also visible on each publication before expanding delivery details.

The report reads current PostgreSQL metadata in a bounded read-only snapshot:
per-channel delivery states/ages, obligations, interactions, current binding
versions, up to 100 retained incomplete/failed Inputs, and legacy delivery counts.
Counts cover all retained rows; the Input list explicitly marks truncation.
It never reads publication bodies, answers, source/artifact refs, recipients,
ACKs, error messages or credentials. Disabled bindings and deleted sources do
not hide their retained failure metadata. The endpoint requires the configured
owner; query parameters cannot select another owner. It grants no content read,
retry, task completion, effect reconciliation or execution authority.

Readiness distinguishes installed components, schema, epoch-1 worker, guarded
source/export handling, Web reader, media planner and terminal widget renderer.
Fixed Broker `GetToolContract` reads additionally compare all four contract hashes
with the kernel schemas. Missing/unavailable Broker observation is unknown;
mismatched schemas and missing adapters remain degraded. Neither the diagnostic
nor a successful tool/adapter construction sets `full_v1_qualified`. Live channel,
native model and full release acceptance remain issue #107.

Prometheus scrapes read durable counts independently of Doctor. Labels are fixed
channel/state enums, without owner or arbitrary IDs. Metrics cover retained
delivery parts, obligations, questions, corrections, retry attempts, oldest
pending delivery and open question ages. Failure emits observation_success=0,
without presenting the previous snapshot as current. Response attempt counters
use validated/conflict/denied; **validated does not prove transaction commit**.
The resume histogram measures the current source/claim/lease verification call,
not time spent waiting for the owner or semantic completion. These process
metrics reset on restart; PostgreSQL counters/history are not reset.

The private operator profile accepts `MaxOpenInteractions`: absent/zero means 16;
1..64 are explicit ceilings. Other values reject startup. Question publication
serializes the owner count across sessions in its existing transaction. Migration100
stores the current owner ceiling and monotonically increasing policy version in
PostgreSQL; an independent row-lock/count trigger also fences earlier epoch-1 SQL
writers that do not contain the new Go check. Trusted startup configures the policy
after validating the bindings; there is no HTTP/model policy mutation. Diagnostics
show the current database ceiling/version and any process/profile mismatch. A full
quota rolls back publication/transcript/outbox/wait together. Exact receipt replay
remains idempotent and resolving a question releases capacity. Lowering the
ceiling never discards an existing question. Other finite format/publication,
claim/lease/source, Broker, approval and budget guards remain in force.

## Operator sequence

1. Keep the private owner profile, database identity, exact numeric bot, binding
   versions and credential file references intact. Inspect diagnostics/Doctor;
   do not infer live readiness from a flag or a Web-available result.
2. Before enabling a different supported profile, run isolated acceptance with
   current source/image/profile hashes. Existing accepted Inputs retain their
   original epoch/methods; adding a method never upgrades their history.
3. To stop new conversations, set the private profile `Mode` to `drain` and
   restart the same compatible binary with
   `orchestrator -communication-profile /private/profile.json`. Keep the matching
   Broker and epoch-1 worker. Reads, pending delivery, existing waits and retained
   ingress receipts continue; fresh messages/replies are refused. No model, tools
   or budget are replayed merely to deliver an existing result.
4. Keep unresolved no-expiry questions and their compatible resume worker. They
   cannot be discarded to allow a schema downgrade. `delivery_unknown` requires
   independent reconciliation; never change it to pending or resend on timeout.
   Known absence may retry only the existing delivery identity. A channel ACK is
   not a human read, verified external effect or task completion.
5. An already stopped taskless Input can use the existing protected `close-failed`
   control. Live workers and uncertain external writes deny. Explicitly
   acknowledging eligible local communication uncertainty abandons the Input and
   retains UNKNOWN, audit and spending; it does not reconcile or replay effects.
6. Schema down is allowed only when its own guards permit it. Migrations089–095/100
   are additive and never backfill legacy results into new publications. Accepted
   explicit/widget/cursor history and owner quota policy prevent destructive down. A compatible binary
   in drain mode is the supported rollback while such history exists. Keep the
   existing backup/deletion-anchor recovery procedures; never restore a snapshot
   merely to erase accepted questions, receipts or unknown spending.

This runbook prepares an operator cutover. It does not authorize or execute a
production deployment. Current proof and limits are recorded in
[the local C08 acceptance](../testing/communication-diagnostics-2026-10-25.md).
