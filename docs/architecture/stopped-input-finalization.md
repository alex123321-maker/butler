# Stopped attempts and conversation availability

Migration102 separates terminal attempt failure from unresolved action outcome.
The explicit epoch1 owner composition installs an Input maintenance collaborator;
models, public payloads and child results cannot install or call it. Legacy
compositions retain their previous policy.

The worker reviews at most eight stopped Inputs per sweep, once per second,
after five seconds without a state change. The sweep has a five-second bound;
each transaction has a 750ms bound and NOWAIT locks. Contention or unavailable
proof defers cleanup. Eligibility requires matching Input/run/session/task and
owner, epoch1 ingress or a prior canonical admission, a stored failure, no live claim and an existing inactive
ownership row. Ownership, session, run and Input are locked against takeover and
owner control. Completed runs are excluded. Blocked Inputs with live pending
approvals, owner questions or protected credential requests are excluded. Ordinary waiting_for_user requests
are not candidates. Missing run/ownership evidence remains fail-closed.

Already failed canonical attempts are also backfilled so older uncertainty
cannot escape the new guard. Their terminal failure is not reopened, and
orphaned waits on an already failed attempt gain no continuation authority.
Blocked candidates take priority over historical backfill and uncertainty
reviews. A failed row transaction does not prevent independent candidates from
being reviewed in the same bounded batch.
Completed runs and actual pending owner waits are excluded before the batch limit
as well as under the final transaction locks, so retained ineligible history
cannot repeatedly consume all review slots.

The finalizer locks journal entries without reading arguments or secrets.
Prepared/allowed entries prove non-dispatch; confirmed/absent entries retain
their owning verdict. Unknown/executing or malformed journal evidence is
uncertain. Tool calls need correlated journal proof, except a finished validation
rejection. Missing receipts and generic runtime completion cannot resolve an
external effect.

Kernel local review supports two explicitly pinned historical revision1
contracts, reproduced from commit `4dff78b5`: atomic `communication.message`
(`f86dbe12464322551ff3d386412b54644fd32227fe0011f1bbac13b7db27b6fc`) and
read-only `web.fetch`
(`18a7d901908ac523b3ab1539d3368a9ea9a4b33ce8190d4d37aa58a4a11ff3d2`).
These are recovery compatibility pins, not permission to execute old contracts.
The optional Communication collaborator invokes the owning Action journal
review inside the same finalization transaction. It requires an already failed,
taskless, isolated Input/run, matching event/owner/session, no claim or active
lease, original canonical intent/binding/hash/fingerprint, and no secret-backed
or extension/converter action. At most twenty unknown effects are examined.

For the pinned message protocol, a consumed UNKNOWN grant/finished dispatch is
required. An exact immutable publication with matching owner, Input, run, call,
versions and canonical accepted receipt proves local publication only. With
the same session/run/Input locks held, absence of that effect's publication
proves local ABSENT: its historical publisher requires those locks and a live
claim at commit. An unrelated publication proves nothing about that effect.
The append-only `stopped_publication_v1` evidence records a content-free digest;
the old grant and dispatch stay consumed/UNKNOWN. Delivery remains separate.
Current message contracts are not automatically covered: later implementations
may own effects outside the publication transaction.

For the pinned read-only contract, `pinned_read_no_write_v1` proves absence of
write uncertainty. The effect and dispatch remain UNKNOWN, without a successful
read receipt. This narrow verified-read map can release stopped write isolation;
it cannot make the old read, run or task successful. Unsupported contracts,
malformed/foreign receipts, arbitrary writes and live execution remain blocked.

The operator-only `cmd/stopped-review` takes up to eight exact Input references,
privately resolves its database connection, and runs this same kernel review.
Its default dry run rolls back every change and reports the predicted
classification separately from the durable one. `-apply` commits verified
evidence. Explicit review skips only the maintenance cooldown. No public/model
endpoint, new approval, synthetic oracle, replay or refund is introduced.

Input records `input_stopped_attempts` and marks the attempt failed. The owning
Communication collaborator verifies that proof, joins Run's failed transition,
closes an open publication obligation with failure and saves a kernel audit in
the same transaction. Failure rolls back every part. Task status/verification
are independently owned and remain unchanged.

Checkpoint, original error, identity, attempts, expiry, usage, reservations,
receipts, grants, publications and delivery history are retained. An effect
verdict can change only through the owning verified review described above. No
provider/runtime is called and no old claim, dispatch or model result is replayed.
The failed Input/run cannot resume or execute an old grant. No owner
acknowledgement or successful completion is manufactured.

Both known and uncertain attempt failures release the session queue. Uncertain
records are reviewed again under the same proof/locks; they become known only
after separately established journal evidence. This never reopens the attempt
or refunds spending. Kernel audit events differ from owner failure-close events.

Core and receiving Broker deny arbitrary non-read effects in the same session
or task tree while isolation remains uncertain, including a newly created Task
in that session. Changing tool, args, call ID or Task cannot evade the guard.
Independent authorized reads retain normal checks.

A fresh canonical `communication.message` can answer the current owner. A
candidate flag from the exact contract, Input claim and pinned destination is
insufficient: the gate also requires the actual live Input, exact intent/args/
contract, admission, methods and publication allowance. Every scoped unknown
effect must be covered by retained isolation proof. Untracked/missing provenance
and an equivalent unknown message deny this exception. Existing blocked-notice/
widget rules remain separately validated. Credentials, schedules and arbitrary
recipients gain no exception. Normal source/export, autonomy, approval, lease
and budget checks continue to apply.

Execution notices expose fixed failure facts without old content. A known later
verdict cannot turn the request into a success. Telegram does not repeat an
identical acknowledged failure when blocked becomes failed; Web still shows
the terminal state. Migration102 and both Core/Broker binaries are required.
Schema rollback refuses to drop nonempty stop records. Any binary rollback must
retain compatible isolation guards; an older Broker cannot replace them.
