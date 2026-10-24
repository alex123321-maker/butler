# Communication kernel (dormant)

Optional explicit owner composition attaches `executionnotice.Store` for
source-free lifecycle facts. These are separate from model publications and
their source/export proof. The protected `communications/states` endpoint can
explain a denied/failed/budget-stopped input without model inference or exposing
its context. Each binding has its own activation and actual delivery receipts.

Canonical answer recovery uses shared Session/run proofs alongside retained Web
readers, with current claim/lease/source checks and post-wait expiry validation.
`interaction_resume_*` failures expose only fixed phases/reasons. The explicit
owner composition has a strict `close-failed` control for already blocked
taskless Inputs, with atomic Input/run failure, closed obligation and one audit.
It rejects live workers and uncertain writes and retains all historical DATA,
errors and spending. It is not a model capability or a retry/UNKNOWN resolution.
An early `run_created/execution_stopped` Web/Telegram failure can also be closed
before communication admission exists: current accepted-receipt/session ownership
and verified Web binding are mandatory, with matching source/session channel.
This creates only the failure audit; no admission or reply obligation is invented.

Implements the backend slice of #99–#100 followed by #103. The product contract
is [user communication](../../../../docs/architecture/user-communication-spec.md).
There is no Bootstrap registration, installed public route, feature flag, model
catalog entry or maintenance loop. The Telegram explicit constructor composes an
opt-in channel worker; normal Bootstrap/legacy behavior and Web remain unchanged.
Merely applying migrations does not admit an explicit Input.

## Ownership and transactions

`Store.AdmitTx` pins epoch 1, supported methods and reply obligation to an already
claimed Input under live session ownership. Message/widget form the baseline.
Photo/report additionally require an explicit transactional `PlanDelivery`
composition. The separate [Telegram adapter](../channel/telegramrich/README.md)
can freeze bounded media/rich parts with current artifact/export checks; this
does not install a full Web/Telegram profile. Child publication is denied.
`verified_task` admission does not demand an unverified result from the model.
Admission verifies the actual run/task binding, live lease/fence and exact private
Input claim. A replacement claim is not usable by the previous caller. Current
goal/mandate authority is rechecked at publication, even after Broker dispatch;
input/source/authority deadlines are rechecked after export-policy waits.
`AdmitRun` derives identity from durable state; an explicitly supplied Core
composition hook runs after source/task preparation and before catalog loading.
Recovery accepts only the same pinned profile. Bootstrap supplies no hook.

The private `app.CommunicationControl` reconstructs actual dispatched
Prepare/Authorize/Execute journal evidence, exact method/args/contract, immutable
destination digest and the current Input claim/lease/fence. The private HTTP
token alone grants no authority. Source refs must belong to selected run sources.
`ApplyTx` is an internal owning operation, never an HTTP/adapter entry point.
Private control uses HTTPS outside loopback and the exact owning Compose HTTP
destination. Dispatch/grant/approval expiry is checked again before committing
the publication transaction; an expired grant rolls back all staged writes.

One transaction inserts immutable publication + canonical transcript message +
per-binding outbox rows + obligation/event. Widget adds the typed question and
Input/Run/Task wait. Locks start with session, then run/task/input/interaction as
needed; the Input owns its claim and checkpoint. The coarse Run state reuses
`awaiting_approval`, but the interaction marker is distinct and creates no
Approval record or authorization. Preference responses remain DATA.

`SourceGuard` is mandatory for current Catalog policy; `ChannelGuard` is mandatory
for every pinned export target at acceptance and send. Future composition must
provide actual per-channel privacy/artifact/export policy. Test guards do not
prove artifact or channel integration. A binding requires an owning verified
actor and that adapter's canonical recipient key; version CAS prevents replay of
an old target. A new/rebound binding never replays backlog automatically.

## Final and delivery lifecycle

Channel-specific compositions narrow `DeliveryStore.ForBinding` to the exact
binding ID/version/channel/recipient. Claim, expired-claim/deadline recovery,
send authorization and finish must not consume or mutate another bot/channel.

Explicit transport requires assistant-final AND run-completed events; abrupt
stream closure cannot succeed. Deltas are internal. Multiple publications do not
conflict with `final:<run>`: explicit final is `attempt_final`, internal, stored
as system DATA; compaction/extraction retain its provenance. Completion preserves
immutable results/effect verification and suppresses implicit final notification.
Completion requires typed terminal proof for both empty and nonempty explicit
finals. After an optional question, a quiet/verified_task run can close its
obligation without an unnecessary or forbidden result publication.

`BeforeFinal` durably spends at most one normal-budget correction. Recovery after
that spend cannot replenish it. A second missing answer blocks execution and
attempts one fixed safe system notice with a stable effect identity. This notice
is separate from the 20 agent-publication limit; the model cannot request the
extra allowance. If no permitted destination exists, a durable failure event
closes the obligation. It never publishes a raw final or loops indefinitely.
Failure to admit or finish the single correction takes this same closure path,
while cancelled/stale ownership cannot create a notice.

`DeliveryStore` implements the existing `deliveryoutbox.ClaimStore`; it does not
introduce another queue or retry policy. Each binding snapshot has independent
delivery status. Claim expiry/ambiguous send -> UNKNOWN, never automatic resend.
Known pre-send policy denial -> failed. ACK handling uses the existing worker;
there is no actual channel sender here. Accepted does not mean sent/read/task done.
Expired/resolved/cancelled questions cannot send their old open-widget payload.
Future adapters still need a current resolved/cancelled projection for channel sync.
One denied export target is stored as `failed/policy_denied` with empty content
and representation; permitted targets proceed. If no target is permitted,
publication fails without fulfilling the obligation. This uses existing outbox
states. Send claims, task/mandate and question expiry are checked after policy
waits too. Respond rechecks current per-channel export policy under binding locks.

## Owner APIs and ordinary replies

`api.CommunicationOwnerControls` is a separately constructed, unregistered backend
handler using existing owner-control authentication. It accepts strict bounded
binding decisions, ordinary replies and typed responses; body/header owner IDs
do not authenticate. Its Web recipient is the authenticated owner's fixed
`web:<owner>` scope. Binding enumeration omits private recipient/proof data.
Telegram Bind/Respond/Reply require a trusted adapter's verified Actor; no current
adapter is changed. Feed/SSE, drafts and rendering remain C05/C06 work.

`Reply` resolves Web publication refs or Telegram's stored sent ACK plus current
binding/version to the canonical session/task. It rechecks current task/constraint/
goal/mandate/source/export policy, and joins the existing Input acceptance
transaction. A stable delivery ID creates one Input; changed content conflicts.
Unproved/ambiguous targets never pick the owner's latest task. Ordinary messages
cannot answer a widget; its explicit typed response path is required. Sessions
are never merged and a reply cannot mint an action grant.
Recognized owner commands use `ControlReply` and the existing Task Controller
outside the waiting FIFO. The controller rechecks the exact binding/ACK/source
target in its own transaction, pins the publication's task revision and dedupes
the owner delivery. Stop requires owner proof, not a still-active execution
mandate. Original channel proof remains private; no channel session is fabricated.
Ordinary `Reply` refuses those commands, and an owner handler without a configured
controller fails explicitly instead of queuing them as model work.

## Waiting and owner controls

`Respond` takes only a verified actor plus interaction/binding/version/typed
answer. First valid response wins across channels. Identical delivery/payload
replay returns the saved answer; a different decision conflicts. Ready markers
survive the original worker's late Finish. No-expiry questions can outlive the
old Input TTL; a response grants a fresh bounded claim window without resetting
spent correction/publication budgets. `ResumeInteraction` requires a fresh owned
claim/lease and rechecks current task/authority/sources; old tools are not run
again from a callback.

`PendingInteraction` distinguishes a conclusive absence (empty ID, no error)
from a failed durable read. Core does not write a generic tool result or resume
the old provider after a failed read or an accepted widget missing its marker.
Its typed state-pending signal retains the original checkpoint for bounded
verification. Input Finish preserves an existing wait or early ready response;
it never grants another tool dispatch/model turn from that signal.

Optional `task.Controller.SetInteractionControls` joins real owner controls.
Pause revokes old execution, retains question/answer DATA; owner resume uses the
Controller's new Input. An unanswered question stays waiting, an answered one
allows that new Input to proceed. Stop/amend close the old question. `Review`
closes expired/stale waits using database time; future composition must schedule
it. It is not a worker holding a live provider stream.
Maintenance selects actionable questions before its batch limit, so older healthy
no-expiry questions cannot starve expired ones. Stop/amend also cancel an open
obligation when its question was already answered but execution has not resumed.

## Storage, rollback and remaining work

Migrations089–090 extend the existing outbox and add bindings, admissions,
publications, interactions and opaque events. Publication identities/targets are
immutable; authorized Catalog deletion erases canonical payload and copied
delivery/question/answer text while retaining identities and hashes. Epoch
filter plus database trigger fence unsupported old claimers. Ordered down is
allowed only with no accepted communication history; never erase live receipts
to downgrade. Disable new admissions and drain with a compatible worker first.

Web rendering (#129), full App/Core/model composition and client acceptance
(#130), widgets/callbacks (#104), owner quotas/config, diagnostic UI and full
rollback/deployment acceptance remain separate work. Telegram media/native
report component evidence is in PR #128; it does not establish full readiness.
Do not enable the profile until the remaining prerequisites are proved.
The isolated #130 composition joins optional `ReplyAdmission` and
`ControlAdmission` hooks inside the existing Input/Controller transactions to
attach private migration093 ingress proof; they cannot grant tool authority.
Migration094 stores fixed Telegram transport clarification receipts with a
durable pre-send claim. Unknown send/finish outcomes never trigger a resend.
The C01/C02/C04 backend acceptance, full fault matrix and historical boundaries
are in [verification](../../../../docs/ai/communication-backend-full-acceptance-20261021.md).
# Authorized Web projection

`WebReader` reads only publications pinned to its current owner Web binding.
`RefreshCursor` checks that binding and reads its monotonic owner-scoped sequence
without loading historical payloads, media or source graphs. SSE invalidation
therefore cannot retain transcript locks behind an answer publication. A pulse
grants no content access; subsequent REST reads still perform the full current
Catalog/export proof, including revocation after the pulse.
Feed/detail/download recheck current Catalog/export and retained media; an
unavailable entry carries no copied payload or manifests. Artifact reads are
limited to the selected publication's required refs and validate digest/size.
The HTTP feed uses one-publication cursor pages, keeping the complete proof for
each returned page through commit. Client-visible history batches are independent
of server transaction size; successive pages retain ordering and omit no history.
`Store.WebAvailable` is a trusted composition capability, default false. New
targets use existing result_ready storage and available API state without send
ACK or human read. Pending history is never upgraded or resent by enabling a
reader. The actual Telegram worker retains its exact binding scope.
Owner Web ingress/replies acquire private immutable epoch-1 proof in their Input
transaction. REST cursor history is authoritative; the authenticated SSE wrapper
only invalidates loaded pages. See user-communication-spec v0.8 and #129/#130.

## C05 widget projections

C07 returns the canonical question and input/options alongside the answer and
separate interaction/publication/Catalog refs. The resumed Core adds fixed kernel
phase guidance separately; payload text stays preference DATA. This avoids relying
on an old provider session or an owner's prompt containing model-generated option
IDs. Current source, ownership, authority and input-claim checks are unchanged.
Owner resume transfers the same question marker to its new Input whether the
answer arrived before or after resume. `ContinuedResponse` reads that exact owned
marker under current task/claim/lease and Catalog checks during preparation of
the new run. It never reconstructs authority from old run grants or public input.

Migration095 adds shared draft CAS, channel-scoped action receipts and a journal
for edits of existing Telegram ACKs. `WidgetActionTx` reuses the current owner,
binding, Catalog, session/run/task/input lock order and `RespondTx`; drafts never
ready the Input. Only explicit submit/answer resolves the question. Privacy
erasure removes derived draft/receipt content. Schema rollback refuses accepted
widget history; use compatible drain. Web exposes the live interaction projection
and separates native edit outcomes from original delivery.

Delivery authorization and known-ACK widget reads lock the publication session
before source/export guards can lock its run. This matches owning response/resume
order; a renderer waiting for the session cannot hold the run and deadlock that
resume. Locks are released before channel I/O. The PostgreSQL lock-wait regression
covers both projection reads and initial send authorization.

Recovery joins its live ownership fence and canonical answer read in one
transaction, including a final claim/lease expiry check. Preparation checkpoints,
run lease refresh and final persistence use the same single-transaction fencing
primitive, leaving capacity for the watchdog on the supported two-connection pool.
The guarded context stays local to that transaction; it cannot authorize a later
independent write. Takeover serialization and all source/Task fences remain intact.
Private owning publication also joins fencing and the publication transaction,
so concurrent dispatched messages/widgets can each use one remaining business
connection. Broker dispatch/claim, approval, SourceGuard and final expiry checks
remain mandatory; this cannot publish through a raw owner Actor.

The owner failure control denies UNKNOWN by default. Its optional exact
`acknowledge_unknown_effects` list can abandon a stopped local communication
prepare Input with the effect still UNKNOWN. It audits the exact IDs and never
updates the Broker journal/grants/evidence, refunds, retries or claims success.
Executing and non-communication uncertain effects always deny. This is an Input
failure acknowledgement, not independent effect verification.
