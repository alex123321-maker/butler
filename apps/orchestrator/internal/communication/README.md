# Communication kernel (dormant)

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
Feed/detail/download recheck current Catalog/export and retained media; an
unavailable entry carries no copied payload or manifests. Artifact reads are
limited to the selected publication's required refs and validate digest/size.
`Store.WebAvailable` is a trusted composition capability, default false. New
targets use existing result_ready storage and available API state without send
ACK or human read. Pending history is never upgraded or resent by enabling a
reader. The actual Telegram worker retains its exact binding scope.
Owner Web ingress/replies acquire private immutable epoch-1 proof in their Input
transaction. REST cursor history is authoritative; the authenticated SSE wrapper
only invalidates loaded pages. See user-communication-spec v0.8 and #129/#130.

## C05 widget projections

Migration095 adds shared draft CAS, channel-scoped action receipts and a journal
for edits of existing Telegram ACKs. `WidgetActionTx` reuses the current owner,
binding, Catalog, session/run/task/input lock order and `RespondTx`; drafts never
ready the Input. Only explicit submit/answer resolves the question. Privacy
erasure removes derived draft/receipt content. Schema rollback refuses accepted
widget history; use compatible drain. Web exposes the live interaction projection
and separates native edit outcomes from original delivery.
