# Atomic result persistence and notification delivery

This contract separates an external effect, a durable attempt result, task
verification, and channel delivery. A provider final event or a runtime
`completed` status proves none of the other three. PostgreSQL is authoritative.

## Finalization transaction

Orchestrator saves a frozen finalizing checkpoint before calling
`completion.Coordinator.Save`. The coordinator requires the matching run/session
ownership token, holds the PostgreSQL shared ownership guard, then locks session,
run, and task in that order. The session lock precedes the transcript revision
trigger. A cancelled/expired/taken-over lease, changed task revision, or inactive
task rejects the transaction.

One transaction inserts the required `assistant_final` artifact, final transcript,
immutable `run_final_results` record, an **unverified** `task_results` row when
attached to a task, stable outbox chunks, auxiliary jobs, and the run's completed
state/transition. The artifact is required for every result. Any write failure
rolls back all of these changes. The task's goal, plan, criteria and semantic
status are preserved. Only the separate owner verification workflow can mark
a task complete; a requested file or outside action is never verified merely
because assistant text was saved.

The result ID is `result:<run_id>`; artifact and final message use `final:<run_id>`.
Retries compare the immutable saved answer. Upgrade recovery reuses an identical
existing assistant transcript rather than appending it twice; conflicting
history fails closed. Result, referenced artifact, and referenced message cannot
be updated. Required objects cannot be deleted independently of the result.

`answer_state=partial` preserves truncated output (including `MAX_TOKENS`), with
an explicit incomplete-output limitation. Every result retains the unverified
task verdict plus the typed action-journal effect/evidence snapshot. Missing
effect evidence remains an explicit limitation. Tool dispatch completion is not
converted into confirmed effect evidence. The stored notification labels the
attempt result and verification limitations separately from the model's text.

A transient final-save failure leaves the finalizing checkpoint. Durable ingress
uses its existing attempt/deadline limits to retry **only persistence of that
answer**. Recovery never starts a model turn or tool call from this boundary.
Authority changes block instead of retrying. A crash or uncertain commit response
is resolved by reading run/result state: a completed run is acknowledged without
re-executing work. Partial-stream delivery is best effort; a failed channel stops
partial delivery for that stream while execution continues. A final event is
never delivered before its result transaction commits.

## Outbox

`delivery_outbox` stores immutable notification content with IDs
`result:<run_id>:<channel>:<chunk_index>`. A worker claims one stored chunk for
30 seconds, makes one channel attempt with a 20-second timeout, and CAS-records
the outcome. It has no model, tool, or execution dependency. Pending delivery has
a 24-hour deadline and at most six attempts. Explicit absent failures use bounded
exponential delay (or bounded channel retry-after); exhausted/permanent failures
remain visible. Restart resumes pending rows. Concurrent workers use row locking
and distinct claim IDs.

| Channel/outcome | Durable acknowledgement | Retry behavior |
| --- | --- | --- |
| Web | `result_ready`: result and transcript available through PostgreSQL REST | No network send is required; optional live events do not claim client receipt/read |
| Telegram accepted | `sent`, positive Bot API `message_id` and matching configured chat | Persist ACK before the next chunk; never repeat that chunk |
| Telegram explicit `ok:false` rejection | `absent` attempt; 429/server rejection returns to `pending`, permanent errors become `failed` | Only the stored chunk may be retried |
| Telegram timeout, lost response, malformed acknowledgement, expired sending claim | `delivery_unknown` | No automatic retry; an earlier ACK remains intact and later chunks are blocked |

Telegram has no idempotency key. Exactly-once user notification is not promised.
If Telegram accepted a message but acknowledgement persistence failed, claim
expiry conservatively records `delivery_unknown`. Stable IDs identify Butler
notifications but cannot deduplicate a Telegram network send. User read is
unobservable in both channels.

Long Telegram text is split deterministically at 4096 UTF-16 units, preserving
UTF-8 runes. Chunks use plain text and independent ACKs; no parse-mode fallback
or internal network retry can duplicate a send. Destinations derive from the
stored session and configured chat allowlist. Redirects are disabled. Channel
errors contain fixed safe codes, never a bot-token URL or raw response body.

`GET /api/v2/results/{run_id}` returns the final record, chunk states, aggregate
delivery state, and auxiliary jobs. Task detail and overview read the outbox
before older partial/approval delivery observations. `pending`, `failed`,
`delivery_unknown`, `sent`, and Web `result_ready` remain separate from task
verification. Existing channel activity is advisory; the outbox is authoritative
for final notification state. These read APIs share the existing private admin
perimeter and do not grant tool/action authority.

## Auxiliary jobs

The same final transaction creates `completion_aux_jobs` for legacy working
cleanup, per-run transient cleanup, and enabled memory extraction/summary work.
The memory job invokes the existing pipeline directly; successful enqueue into
Redis is not treated as durable completion. PostgreSQL jobs have five-minute
claims, four-minute processing deadlines, bounded delays and eight attempts.
Expired claims can retry auxiliary processing; exhausted failures are retained.

Auxiliary execution is at least once and uses the memory pipeline's duplicate
and summary-coverage policy. It can rebuild memory, never execute user tools.
Cleanup matches the completed run ID so a delayed job cannot erase a newer run's
working state. Durable task working state is retained. Redis/memory/extraction
failure changes the auxiliary job only and cannot undo the saved result or an
external effect. The legacy Redis queue remains available for operator pipeline
jobs, separate from the durable post-run completion queue.

## Validation

Lab-only PostgreSQL tests exercise atomic rollback after artifact insertion and
after the completed-state update, restart after save-before-delivery, same-run
persistence recovery without model/tool repetition, Unicode chunks with stored
ACK/429/unknown outcomes, concurrent claims, expired sending claims, stale fence,
expired ownership, cancellation/version changes, and recoverable memory failure.
Use `BUTLER_TEST_COMPLETION_POSTGRES_URL` for the coordinator/outbox integration
suite and `BUTLER_TEST_POSTGRES_URL` for durable input lifecycle integration.
