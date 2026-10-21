# Communication kernel (dormant)

Implements the backend slice of #99–#100 followed by #103. The product contract
is [user communication](../../../../docs/architecture/user-communication-spec.md).
There is no Bootstrap registration, public route, feature flag, model catalog
entry, maintenance loop or channel sender. Existing Web and Telegram adapters
are unchanged. Merely applying migrations does not admit an explicit Input.

## Ownership and transactions

`Store.AdmitTx` pins epoch 1, supported methods and reply obligation to an already
claimed Input under live session ownership. Only message/widget are supported;
photo/report have contracts but cannot be admitted. Child publication is denied.
`verified_task` admission does not demand an unverified result from the model.

The private `app.CommunicationControl` reconstructs actual dispatched
Prepare/Authorize/Execute journal evidence, exact method/args/contract, immutable
destination digest and the current Input claim/lease/fence. The private HTTP
token alone grants no authority. Source refs must belong to selected run sources.
`ApplyTx` is an internal owning operation, never an HTTP/adapter entry point.

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

Explicit transport requires assistant-final AND run-completed events; abrupt
stream closure cannot succeed. Deltas are internal. Multiple publications do not
conflict with `final:<run>`: explicit final is `attempt_final`, internal, stored
as system DATA; compaction/extraction retain its provenance. Completion preserves
immutable results/effect verification and suppresses implicit final notification.

`BeforeFinal` durably spends at most one normal-budget correction. Recovery after
that spend cannot replenish it. A second missing answer blocks execution and
attempts one fixed safe system notice with a stable effect identity. This notice
is separate from the 20 agent-publication limit; the model cannot request the
extra allowance. If no permitted destination exists, a durable failure event
closes the obligation. It never publishes a raw final or loops indefinitely.

`DeliveryStore` implements the existing `deliveryoutbox.ClaimStore`; it does not
introduce another queue or retry policy. Each binding snapshot has independent
delivery status. Claim expiry/ambiguous send -> UNKNOWN, never automatic resend.
Known pre-send policy denial -> failed. ACK handling uses the existing worker;
there is no actual channel sender here. Accepted does not mean sent/read/task done.

## Waiting and owner controls

`Respond` takes only a verified actor plus interaction/binding/version/typed
answer. First valid response wins across channels. Identical delivery/payload
replay returns the saved answer; a different decision conflicts. Ready markers
survive the original worker's late Finish. No-expiry questions can outlive the
old Input TTL; a response grants a fresh bounded claim window without resetting
spent correction/publication budgets. `ResumeInteraction` requires a fresh owned
claim/lease and rechecks current task/authority/sources; old tools are not run
again from a callback.

Optional `task.Controller.SetInteractionControls` joins real owner controls.
Pause revokes old execution, retains question/answer DATA; owner resume uses the
Controller's new Input. An unanswered question stays waiting, an answered one
allows that new Input to proceed. Stop/amend close the old question. `Review`
closes expired/stale waits using database time; future composition must schedule
it. It is not a worker holding a live provider stream.

## Storage, rollback and remaining work

Migrations089–090 extend the existing outbox and add bindings, admissions,
publications, interactions and opaque events. Publication identities/targets are
immutable; authorized Catalog deletion erases canonical payload and copied
delivery/question/answer text while retaining identities and hashes. Epoch
filter plus database trigger fence unsupported old claimers. Ordered down is
allowed only with no accepted communication history; never erase live receipts
to downgrade. Disable new admissions and drain with a compatible worker first.

Owner APIs, generic reply/ACK mapping and ambiguity resolution, Web/Telegram
renderers and callbacks, media/report validation, native model behavior, owner
quotas/config, diagnostic UI and full rollback/deployment acceptance remain
separate work. Do not enable the profile until those prerequisites are proved.
See [verification](../../../../docs/ai/communication-core-acceptance-20261021.md).
