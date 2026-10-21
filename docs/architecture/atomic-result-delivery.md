# Atomic result persistence and notification delivery

This contract separates an external effect, a durable attempt result, task
verification, and channel delivery. A provider final event or a runtime
`completed` status proves none of the other three. PostgreSQL is authoritative.

## Planned explicit-publication profile

The dormant backend foundation now implements message/widget transactions,
typed internal finals and the existing outbox outcome policy. It is not composed
by Bootstrap. See [scope and evidence](../ai/communication-core-acceptance-20261021.md);
legacy delivery remains the enabled path.

Explicit completion requires typed assistant-final/run-completed proof for both
empty and nonempty final text. A saved publication cannot turn an interrupted
terminal stream into a completed attempt. A failed single correction closes its
still-open obligation without another inference or raw-final fallback; quiet
questions return to a satisfied obligation after their valid owner response.
The backend follow-up verifies each publication/response write with failed storage
responses and abrupt worker death, plus post-commit ACK loss. A denied export
target uses existing `failed/policy_denied`, empty content and representation;
other permitted targets proceed. All-target denial still rejects the publication.
Current claim/task/mandate/question deadlines are rechecked after policy waits.
See [the backend matrix](../ai/communication-backend-full-acceptance-20261021.md).

[User Communication](user-communication-spec.md) specifies a future admission
profile that preserves immutable attempt results and verification while removing
implicit notifications from model finalization. Explicit publications receive
independent identities and transactional multi-channel outbox entries, including
mid-run progress and widgets. Existing legacy and verified_task semantics below
remain authoritative until the documented migration; communication tools cannot
bypass the owner-configured verified_task gate. Reuse the existing UNKNOWN,
channel-only dispatch and frozen-result recovery contracts.

The planned foundation C02 separates publication transcript entries from the
immutable internal attempt final before exposing a working communication method.
`AppendFinalTx` must select its own final identity/kind rather than every assistant
message in the run; multiple publications cannot conflict with final persistence.
An explicit terminal may have no text after publication or a quiet run. Legacy
finalization below remains unchanged until implementation of that profile.
Replay, user history and memory consumers preserve the typed distinction.
The isolated C02/C03 message flow proves publication, further tools and final
without a duplicate before reports/widgets; production enable still needs the
complete supported profile and separate owner authorization.

For a planned blocking widget, publication/continuation, owning Input checkpoint,
Run/Task wait intent and obligation commit together before delivery. Response
readiness is monotonic across the old worker's late Finish; fresh ownership gates
the unique resume. See the interaction handoff in User Communication. These are
target contracts, not claims that current approval recovery handles widgets.

## Finalization transaction

Orchestrator saves a frozen finalizing checkpoint before calling
`completion.Coordinator.Save`. The coordinator requires the matching run/session
ownership token, holds the PostgreSQL shared ownership guard, then locks session,
run, and task in that order. The session lock precedes the transcript revision
trigger. A cancelled/expired/taken-over lease, changed task revision, or inactive
task rejects the transaction.

The typed permanent journal reader completes and closes its queries under the
ownership guard before the result transaction opens. This sequencing needs two
pool connections, even when the guard occupies one. Final results retain the
effect/evidence snapshot observed at save time; later reconciliation belongs to
the permanent journal and does not rewrite the immutable saved result. A legacy
technical tool completion with no journal coverage remains explicitly unverified.

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

`answer_state=partial` preserves truncated output (`MAX_TOKENS`, `length`, or
explicitly incomplete/filtered/cancelled final events), with
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

Telegram suppresses model draft fragments and per-tool chat messages. Chat
actions can still indicate work; stored final chunks continue through the same
acknowledged outbox with unchanged retry/unknown-delivery rules.

Artifact payload text is normalized to valid UTF-8 without rewriting identity
fields; summary truncation respects rune boundaries. Migration070 admits the
existing browser_capture/image_data_url shape. It changes storage validation,
not browser selection, execution authority or permission to send an image to a
model. The down migration retains captures and fails atomically while such
records make the older constraints incompatible.

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

Task detail renders these existing fields separately: saved complete/partial
response, immutable action/evidence snapshot and current notification delivery.
It does not infer a mutation from a confirmed read effect or treat channel ACK
as user read. Unknown or unavailable state stays explicit. The overview shows
the task's criteria and separate attempt links; no saved answer verifies them.

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

## Owner-configured verified task delivery

Migration069 adds an explicit `verified_task` mode, configured by the authenticated
owner on an initiative Source. Existing Sources and tasks keep `attempt` delivery.
The Task module freezes this mode at root admission and inherits it unchanged to
actual children. Model tool arguments and public owner-task JSON cannot select it.
A Source cannot combine this mode with `DigestChannel`.

Completion still saves each immutable technical final, artifacts, transcript,
effects, unverified task result and auxiliary jobs atomically. For this mode only,
it suppresses the external per-run notice. Technical success, child reports,
activation and conversion do not produce the semantic owner notification.

Only current authenticated owner completion of the root with an independently
verified `task_results` record creates one `task_verified_notifications` row in
the same Task transaction. The original exact task/constraints revision, complete
unredacted completed attempt, current owner/session and assignment must agree.
Partial answers, deleted sources, unrelated or stale results and cancellation
cannot enqueue a verified-success notice. Store completion replay and Controller
delivery replay retain the exact result/notice identity. Failure to enqueue rolls
back completion. This is an owner decision; no model final auto-verifies a task.

The frozen content contains the opaque task link and confirmation status, never
the original source, owner proof summary or model response. Authenticated task
detail exposes its separate delivery state. Web starts `result_ready`; Telegram
starts `pending`. Task owns this outbox. The existing channel-only dispatcher
claims, authorizes against current task/owner/privacy/assignment and fresh actual
PostgreSQL time, and records channel ACK. It has no executor/provider capability.
Explicit recipient absence permits only bounded retries of the same row. Expired
sending claims and acceptance without persisted ACK become `delivery_unknown`
and never resend or rerun work. Exactly-once recipient delivery is not promised.

The default attempt outbox and historical notices retain their original semantics.
Rollback of069 refuses to discard any opted-in task or stored notice history.

The ordinary outward delivery sink applies the immutable Task mode before the
channel-delivery observer. Opted-in assistant drafts/finals, all tool lifecycle
progress and ordinary status do not make HTTP channel requests or mint `sent`
`channel_delivery_events`. Approval requests and explicit policy blocked/waiting
interventions remain available. Core task/run/tool/activity/effect history stays
observable independently. The final task outbox uses the raw channel sender after
its own authorization, not the progress filter.

Queue and send additionally invoke the trusted owning Catalog's historical
`CheckRunSourcesTx` in the same transaction, including restore anchor and exact
source/transcript/delegation pins. An absent adapter denies this mode. Correcting
an original source invalidates completion/send even when the old technical final
remains unredacted. This source-only read grants no current completed-run lease
or model/tool execution permission.
