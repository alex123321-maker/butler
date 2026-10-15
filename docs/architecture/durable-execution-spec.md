# Durable ingress, session ownership and approval continuation

This contract replaces request-bound execution and process-local approval waits
for normal HTTP, gRPC and Telegram ingress. PostgreSQL owns acceptance, ordering,
execution checkpoints, expiring ownership and decisions. Redis and provider
session references do not authorize dispatch or reconstruct an unknown result.

The input worker has a ten-minute default execution bound. A declared runtime
profile may use the explicit startup constructor `NewServiceWithExecutionTimeout`
with a positive bound no greater than forty minutes. This configures a new
service before workers start. Earlier caller, accepted input, task, mandate,
claim, ownership and source deadlines remain authoritative; retries never extend
them and existing services receive no timeout mutation.

## Planned interaction continuation extension

[User Communication](user-communication-spec.md) specifies durable non-approval
widgets and authenticated cross-channel replies. Its future waiting_for_user
lifetime may be unbounded until answer/cancel, independently of the ordinary
24-hour input acceptance bound below. Implement this through owning Input/Task
transitions and preserved budgets, not by changing old deadlines during replay.
First accepted submit and one continuation commit atomically; preference replies
never authorize actions. Existing exact approvals remain separately enforced.

## Acceptance and ordering

`POST /api/v1/events` returns HTTP 202 with `accepted`, `input_id`,
`receipt_status`, `session_key` and optional `task_id`/`run_id`. `Location` points
to `/api/v3/inputs/{input_id}`. gRPC `SubmitEventResponse` has the same additive
receipt fields. A conversation may omit `task_id`; acceptance does not invent a
task, and a run ID can appear after acceptance. The receipt page is
`/inputs/{input_id}`. Older synchronous executor adapters retain their existing
response fields for compatibility.

The durable identity is `(session_key, source, idempotency_key or event_id)`.
Repeating it with different payload, event type, owner or task is rejected.
Session-row admission serialization precedes assigning `session_sequence`, so
an uncommitted earlier input cannot surface behind an already claimed later one.
Only the oldest unfinished input per session can execute. Waiting or blocked
heads block later inputs. Across sessions the oldest ready head wins, with
`SKIP LOCKED` allowing independent execution. A busy session remains queued and
does not consume the recovery retry allowance. Queued deadlines remain visible
even behind a blocked head.

The app starts two independent workers, polls every 250 ms, claims for 60 seconds
and renews at one third of that duration. The worker context belongs to the app,
not to an HTTP connection or Telegram update. Execution is bounded to ten
minutes; acceptance expires after 24 hours. At most three execution claims are
allowed, including the initial attempt and crash recovery. An owner decision
queued for continuation does not consume this allowance; a crash after that
continuation is claimed does. Repeated crashes cannot renew the allowance.
Receipts expose attempts, deadline, checkpoint and error code. Automatic retries
never replay an uncertain external action; blocked work requires owner review.

Independent claims acquired or renewed at different times also expire at different
times. Available worker width permits simultaneous eligible sessions but does not
synchronize crash-recovery eligibility. A parallel acceptance fixture must observe
all relevant old claim expiries using the PostgreSQL clock before measuring
overlap; it must not rewrite TTLs or infer serialized workers from a fast run that
finishes while another claim is still live.

For a bounded delegated input, the original kernel event ID and refs-only payload
remain immutable. Ordinary workers replace only the execution event's run
deduplication key with the accepted `input_id`. Core preparation and recovery use
the owning input store's typed `DelegatedInputVerifier` to prove this mapping
against the current private claim and worker fence. The verifier locks
ownership → session → run → task → accepted input on one connection and checks
the actual owner, accepted/run IDs, pinned task revision, original event fields
and exact contract SourceRef. Fresh PostgreSQL clock checks reject lease, claim
or admission expiry after waits. A missing verifier or substituted mapping is
denied; strict legacy direct-core events retain their exact event key. Neither
path turns refs into authority or skips current Catalog/goal/mandate checks.

## Session authority and context

Session Service uses `session_ownership` and PostgreSQL clock time. Acquisition
after release/expiry creates a new random lease ID and strictly larger fencing
epoch. Renewal cannot revive an expired lease, and release of an old lease cannot
remove its successor. `Lease.fence_epoch` is additive in the session contract.

Execution contexts carry the session/run/lease/epoch token. Authority writers
check it and hold a shared ownership-row lock while writing; takeover acquires
an exclusive lock. Run state/ref, transcript adapters, task checkpoints/results,
working memory, artifacts, activity, delivery records and channel delivery are
guarded. Broker independently rechecks live authority before secrets and dispatch.
Controller operations have separate authority and do not reuse worker tokens.

One `ownership.Authority` instance must be shared with the lease manager and all
workers using a pool. A guarded operation needs a guard connection plus a business
connection. The pool must have at least two connections. Guards are limited to
half the pool; lease mutations wait outside the pool for local guards to finish,
preventing row-lock waits from consuming the final business connection. Two
concurrent sessions and takeover have been exercised with `MaxConns=2`.

Only identity validation and durable admission precede ownership. Transcript,
verified summary/tail, retrieval, task revision, full protected task constraints
and model context are loaded after acquisition. Safe preparation recovery omits
the current run's already-recorded user input from replay and does not append it
again. Task working state survives a run's final answer.

## Checkpoints and recovery

The receipt preserves prepared Butler input/tool definitions separately from
changing stage data. Recoverable boundaries include accepted input, run creation,
queued/acquired/preparing, an exact suspended approval, a durable known tool
result and a saved finalizing response. Recovery retains the original input,
run, tool-call and action IDs; it obtains new ownership and a fresh dispatch grant.

Before external dispatch, `tool_dispatching` records intended IDs and the private
grant reference. After durable transcript persistence, `tool_result_recorded`
records the outcome stage. All known action results from the same run feed a
fresh provider session without dispatching any tool again. Further live tool
results use the current transport's normal submission path, retaining native
provider context. An absent result or `effect_unknown` becomes
blocked and never starts autonomous model retries. A saved finalizing response
can finish persistence without asking the provider again. Existing assistant
transcript content is checked before appending, preventing a duplicate final row.
Interrupted model execution with no safe boundary is conservatively blocked.
The [bounded model retry](model-retry-policy.md) adds one durable queued recovery
after a trusted nominal model-turn timeout. It obtains fresh ownership, sources,
catalog and model admission, preserves all spent reservations, and denies replay
of equivalent confirmed non-read proposals. Its stored started marker prevents
another request after a crash during the retry.

Final result persistence, required artifacts, run completion, notification outbox
and auxiliary jobs commit atomically under the current PostgreSQL ownership
fence. A transient final-save failure may retry only the frozen answer; tool and
model execution never replay from this boundary. A completed run does not imply
verified task completion or successful channel delivery. Telegram unknown send
is retained without automatic retry; Web readiness means stored availability.
See [atomic result and delivery](atomic-result-delivery.md) for channel ACK,
claim expiry, stable chunk IDs, artifact failures and memory recovery semantics.
Exactly-once outside effects or non-idempotent notifications are not promised.

## Durable exact approval

Approval Service atomically saves the exact action binding, task/constraints
revision, owner, permitted reply channel, expiry, version-1 data-only continuation,
run awaiting state, input checkpoint and approval event. The worker then returns
and releases the session lease. After committing the continuation, it cancels
the run's private provider context, including pending native calls/signatures.
Recovery under a fresh fence discards any remaining local provider context before
starting a new one with no provider reference. Recorded tool observations remain
user-level data; they are never promoted into system instructions or orphan native
function responses. Every known result, including failure details, is replayed;
new live tool results use the new context's exact call/signature correlation.
`ApprovalGate` is only a compatibility adapter for
older direct callers; production ingress does not require a live waiter.

The binding pins canonical argument and contract hashes, resource, credential
references/metadata revision, task revision and constraints revision. Stored
display arguments are checked against the same canonical hash. Continuations
contain Butler context and the tool request; they do not serialize a Go stack or
grant authority through a provider reference. Private grant IDs stay in system
checkpoint data and are omitted from receipt APIs and model inputs.

Owner replies are durable `approval_decision_inputs` with source/delivery identity.
The decision and input-ready transition commit together. A response received
during notification is preserved: worker completion sees `approval_ready` and
queues continuation instead of overwriting it with waiting. Competing yes/no
decisions use row locks and pending-state CAS; only one wins. Repeat delivery is
idempotent and cannot dispatch twice. The saved channel is enforced; a different
channel cannot turn an approval into another execution path.

Cancellation, changed revisions, expiry and terminal runs invalidate permission.
A bounded controller sweep expires invalid pending decisions without an owner
reply and queues the blocked continuation. The sweep can only expire invalid
actions. Denial/expiry blocks the task and receipt while retaining run history;
it is not a technical failure of the system. A later lease still revalidates the
binding before authorization. `approval_actions` records waiting, ready,
dispatching, completed or blocked continuation state.

## Reply identity and operations

Web approval controls require a separate owner-control Bearer proof. Provision
`BUTLER_OWNER_CONTROL_OWNER_ID` and exactly one of `BUTLER_OWNER_CONTROL_TOKEN`
or `BUTLER_OWNER_CONTROL_TOKEN_FILE`. The token must contain at least 32 bytes.
Values are operator-owned environment/file settings, excluded from database
configuration overrides. `X-Butler-Actor` or a body owner ID is not proof.
Extension and private Broker service tokens have different scopes. Missing owner
authority denies web decisions even on a private LAN. Token comparison is constant
time; errors/logs/model context never include token material.

Generate and inspect the owner token on the lab terminal, never in a chat message.
Use a restrictive token file or deployment environment, and match the configured
owner to the saved web session owner. The Approvals page keeps the supplied token
only in component memory and clears it when leaving; it does not use browser
storage. A Telegram approval requires the numeric `From.ID`, an allowed chat,
the exact saved Telegram session and the saved channel. General web ingress and
other admin APIs retain their existing authentication scope; this token does not
claim to replace the future shared owner identity subsystem.

Migrations 032 and 033 preserve old run/transcript history. Legacy tool waits
without exact intent/continuation are expired and associated tasks blocked; they
require explicit re-preparation. The migration never invents owner approval or
execution evidence. Migration 031 separately owns Broker action authorization.
Rolling back 032/033 removes new receipt, ownership and continuation tables;
back up that state before an operational rollback. It preserves historical runs,
messages and approval decisions and does not revive expired legacy permissions.

## Validation evidence

Lab PostgreSQL suites exercise HTTP disconnection after 202, delivery dedup,
session FIFO/fairness, expired claims/stale finish, waiting restart with same IDs,
fast decisions during notification, competing yes/no, wrong owner/channel,
cancellation/constraint changes, expiry without a reply, crash after user transcript,
known-result recovery and uncertain dispatch without replay. Lease tests cover
expiry/takeover, stale renewal/release/writes and two-connection concurrency.
Web fixtures cover the real approval envelope, owner proof headers, absence of
token browser persistence, and an independently observed input receipt.

Transport/tool fakes isolate lifecycle faults; the separate Broker suite uses
actual private gRPC/HTTP authority, PostgreSQL approval and Chromium credential
effects. Production-model login and real remote-site reconciliation are separate
deployment validations and are not implied by these tests.

Accepted inputs retain random trusted action slots in a separate
`checkpoint_json.action_slots` object before Broker preparation. Later
`checkpoint_data` replacement preserves it; approval continuations retain the
logical reference and permanent effect ID. New identical proposals get fresh
slots. Cross-run reuse requires an explicit trusted parent-slot link and unchanged
task/constraints/argument binding. Dispatch checkpoints include effect IDs;
result checkpoints retain typed effect state and evidence references. See the
[external effect journal](external-effect-journal.md) for cancellation ordering,
recipient guarantees and evidence-based reconciliation.
