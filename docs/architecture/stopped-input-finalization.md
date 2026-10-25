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

The finalizer locks journal entries without reading arguments or secrets.
Prepared/allowed entries prove non-dispatch; confirmed/absent entries retain
their owning verdict. Unknown/executing or malformed journal evidence is
uncertain. Tool calls need correlated journal proof, except a finished validation
rejection. Missing receipts and generic runtime completion cannot resolve an
external effect.

Input records `input_stopped_attempts` and marks the attempt failed. The owning
Communication collaborator verifies that proof, joins Run's failed transition,
closes an open publication obligation with failure and saves a kernel audit in
the same transaction. Failure rolls back every part. Task status/verification
are independently owned and remain unchanged.

Checkpoint, original error, identity, attempts, expiry, usage, reservations,
receipts, grants, effects, publications and delivery history are retained. No
provider/runtime is called and no old claim, dispatch or model result is replayed.
The failed Input/run cannot resume or execute an old grant. No owner
acknowledgement, effect reconciliation or successful completion is manufactured.

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
Rollback refuses to drop nonempty stop records and their guards.
