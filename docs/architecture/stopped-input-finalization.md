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

Kernel local review supports explicitly pinned historical revision1
contracts, reproduced from commit `4dff78b5`: atomic `communication.message`
(`f86dbe12464322551ff3d386412b54644fd32227fe0011f1bbac13b7db27b6fc`) and
read-only `web.fetch`
(`18a7d901908ac523b3ab1539d3368a9ea9a4b33ce8190d4d37aa58a4a11ff3d2`).
These are recovery compatibility pins, not permission to execute old contracts.
The public GET-form compiler from commit `1e24a910` is additionally pinned as
`browser.submit_get_form` / revision1 / contract hash
`504555e5109e2f7478d87fc21a5598c28f1f27b3aaa58830851675cab9bff90f`.
Its reproduced contract is retained as a test fixture. Under the same original
binding/owner/terminal-fence checks and exact read-only/read-result policy it
can establish only no-write uncertainty, like the pinned web reader. It does
not import revision2 pre-navigation absence semantics into an old failed read.
The credential-free revision3 `browser.navigate` contract reproduced from
49ada352 is pinned separately as
`bf151ae84aaff52f3c3c22c0634e9bc7f48b49c551a4b6777d52d5fdf1c429f2`.
Its exact read-only/read-result policy can establish only no-write uncertainty
under the same terminal owning fences. The historical read, effect, consumed
grant and dispatch stay UNKNOWN and the Input/run stay failed. This does not
apply revision4's pre-navigation ABSENT contract retrospectively. Unsupported
hashes/revisions, credentials, live leases and changed policies remain denied.
This compatibility review may run through the standalone operator; no policy
reset or service restart is required to commit its append-only proof.
The ordinary-text revision2 `browser.prepare_fill` contract reproduced from
00cbdf04 is separately pinned as
`4b4d58b1ddfadd6a2c56e5b2596fe85d2f4edad96105f52304e9c6fa3546aff3`.
Its exact frozen-local policy and matching pinned context/URL prove only absence
of an external write. `pinned_preparation_no_write_v1` retains UNKNOWN local field
state, dispatch, spending and failed Input/run. Credential-backed preparations,
opaque live fills, other hashes/revisions/policies and active execution do not
qualify. This does not apply revision3's new pre-input ABSENT verifier backwards.
The budget loop independently recognizes that exact immutable no-external-write
proof only after the original stopped attempt is classified known. It rechecks
the complete owning/consumed-dispatch provenance for every scoped uncertainty;
classification alone cannot release the guard. Both Core and Broker need this
reader. Local UNKNOWN and all charges remain; normal current permission checks
still govern each new proposal.
Historical review also supports the revision1 `credentials.request_access`
contract (`35ee548617ec6944e5bfc3fe85c9d4962d1551f5b8393decfa2b97ff0f45c9df`).
The original canonical intent, consumed UNKNOWN grant/finished dispatch and
terminal taskless fences remain mandatory. Credential Control independently
checks the exact request owner/session/run/call/versions, an expired request
and browser context, no parent, no notice dispatch/ACK, and exactly one owning
`requested` audit with no approval or release audit. If the expired browser row
was pruned, only its confirmed, credential-free original `browser.open` journal
and digest-matched saved result can establish the original expired context.
An existing foreign/live context cannot use this fallback. Reconstructed original
metadata arguments must match the pinned argument hash. Only creation becomes
CONFIRMED, with digest-only `stopped_expired_access_request_v1` evidence; grant
and dispatch remain consumed/UNKNOWN, the request stays expired and Input/run
stay failed. No notice, consent, secret release, form submission or login result
is inferred. Missing/foreign/mutable/unmatched evidence retains uncertainty.
Separately, absence of any request with that exact original call ID proves local
ABSENT under those same terminal session/run/Input locks. The pinned creator
takes shared session/run locks, requires a nonterminal run and atomically commits
its request/audit, so no late creation crosses the review. A same-call row in any
run conservatively prevents absence. Digest-only
`stopped_access_request_absence_v1` evidence cannot borrow another call's request,
release secrets, deliver notices, reopen the failed Input or replay a dispatch.

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
The reproduced revision2 contract from00cbdf04 is additionally pinned as
`ecc90b761ddf163c0e14c0f75f99fa00e7a4893e0b9f4ded61613a86f8604a2b`.
Its reviewed publisher retains the same fenced atomic publication/outbox
transaction, so the same exact owning receipt/absence checks apply. Revision1
with this revision2 hash, another hash/revision, or an unknown writer does not
qualify. This pin does not authorize execution of an obsolete contract.

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
