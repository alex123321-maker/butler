# Workspace execution composition

`Provenance` joins actual workspace metadata to the owning memory Catalog on the
same PostgreSQL pool/transaction. It grants no tool, runtime, budget or credential
permission. Exact dispatch admission and the private gRPC file runtime are now
implemented and tested through the actual Broker Server/router. Application
bootstrap and operator/owning readiness are now composed under default-false
operator configuration; owner reconciliation and main rollout remain pending;
the owning context projection and exact human approval in these tests are
explicit fixtures. No model tools or production flags are enabled by this change.

`DispatchStore` implements the Broker admission/claim hooks. Migration067 stores
immutable approved operation/path/body hash/file CAS, original input-claim hash
and original source context; no body, host root or reusable service token enters
that history. The same action-claim transaction checks actual current worker,
task, input/checkpoint, source graph and permanent intent, reserves the existing
shared budget, acquires/claims the exact `workspace:<ID>` resource and creates one
private ticket. Any denial rolls back all permission/reservation/lease changes.
A stale file snapshot is rejected under the resource lock before that transaction
commits. Tickets expire within the original input/grant/worker/budget bounds and
the maximum resource lease of30 seconds. They do not renew a worker.

`Contracts` exposes only canonical `workspace.create/read/write` DATA arguments:
an opaque namespace, relative code path, bounded UTF-8 text and exact previous
file version/hash/size. Strict parsing rejects duplicate/extra/case-alias fields,
raw credential patterns, private paths, host roots and caller-supplied authority.
Text is bounded to64 KiB. Creating a copy executes no code and copies no host
environment. Reads require independent current access to the cumulative original
graph; writes preserve provenance without publishing accepted memory.

`Runtime` authenticates exactly one private service header and ticket, then
rechecks correlated canonical contract/args/fingerprint, actual permanent journal,
worker/task/input/source/budget/resource before committing the ticket's one-use
start. A fixed private os.Root and the real workspace Store/Observer perform files.
Independent physical evidence, immutable source context, file metadata and write
lease consumption commit together. Reads release their held read lease only after
current final checks. Generic/lost/failed acknowledgements stay unknown; tickets
and actions never replay. An uncertain write retains the existing physical pending
checkpoint and quarantine. Runtime privacy cleanup runs only the separately bounded
Catalog deletion job, which cannot reconcile or resume execution.

The Broker `ResourceAdmissions` routes workspace and browser namespaces to their
separate exact guards; one cannot substitute for the other. The workspace executor
obtains only an already-committed ticket and passes it as private gRPC metadata
through the existing router. No credential-bearing request is supported. Owning
`workspace_ready` gates exact contract discovery/preparation and the original
input-claim hash is pinned privately to the grant. The application does not yet
set that readiness or register these contracts unless explicitly enabled by the
operator. Exact authenticated installed readiness checks canonical hashes, root
identity/private permissions, source/deletion anchor and intact immutable schema.
Fixed Unix socket or literal loopback is supported, with no proxy/DNS/LAN bearer
route. See [the runtime application](../../apps/tool-workspace/README.md).

`Runtime` holds exclusive Linux `workspace.RootOwner` ownership of the actual
directory inode before opening Store/Observer/privacy handles. Contention and
unsupported platforms deny startup with a fixed safe error. Before constructor
success, each of the three actual opened handles must match the RootOwner's
locked directory inode, as well as its current
named path. Temporarily replacing the root and restoring it before the final
named-path check cannot leave a Store, Observer or privacy executor anchored to
a different directory.

No lock/PID/temp file is created; independent roots retain concurrency. Readiness
and Execute check the current named private root against that held inode; Store admission
repeats this after authority/resource waits and privacy erasure checks it around
physical work. A lifecycle guard
drains entered Execute/readiness/privacy work before Close releases ownership;
Close is idempotent and a closed runtime denies future entry. Process death
releases only the operating-system lock, preserving PG resource/effect history
and quarantine. This is a prerequisite for explicit owner discard, not itself an
owner reconciliation decision or a new tool permission. Actual process/SIGKILL and PG
pool2 Close-versus-Execute regressions run with
`scripts/dev/workspace-lab.sh <isolated-agent-root> ownership`.

`PrepareTx` runs before effect/budget/resource/workspace locks. It requires the
current actual worker/session/task/constraints and deletion anchor, reads only
roots of the actual run's complete pinned graph and the latest immutable
workspace context, and produces private `SourceContext` metadata. Ancestor pins
alone do not grant a child selectable ancestor bodies. No model argument selects
owner, task, source scope, original graph or evidence.

`CheckTx` repeats that held source set and a fresh database clock; changed
ownership, source scope/revision/deletion or workspace context denies access.
Only this transaction's own fresh invisible publication may advance its prepared
context once. A committed old context cannot be reused as fresh publication.
Contexts retain only the real worker token/deadline/cancellation: owner actors and
auxiliary job checkers cannot cross this boundary.

After independent physical confirmation, `ConfirmTx` creates empty-body
task-scoped DATA using Catalog.CreateTaskDerivedTx and records migration066's
immutable context on the file metadata/resource transaction. The new context
inherits the previous copy and current run graph. It contains only provenance
and physical-observation metadata, with no copied source/code body, host root,
credentials, accepted fact or policy. Every later run must independently check
that original graph before releasing existing file bytes. File/resource admission
still has to validate the exact approved operation/body/path/snapshot.

Catalog deletion/revocation transitively tombstones these derived contexts. Its
same transaction blocks the affected workspace and queues physical erasure.
The durable queue has an exact foreign key to that workspace's immutable source
history. `PurgeDeleted` checks the deletion anchor, committed tombstone, blocked
copy and queue under locks, then calls a fixed operator `workspace.DeletionRuntime`
to erase only that opaque namespace. The runtime uses private os.Root containment,
does not follow a namespace symlink or accept a host path, and syncs the root even
when an earlier attempt already removed the copy. Failed/lost acknowledgements
keep the job pending. Retry/restart is safe; erasure never unblocks execution.
This privacy job supplies no owner reconciliation or worker recovery decision.

Actual lab tests exercise pool2, resource claims, filesystem metadata CAS,
independent Observer, source publication, a new run, delete/revoke erasure,
failure after rename, immutable lineage, source-row wait expiry, owner/job scope
shortcuts and lost erasure acknowledgement with a restarted runtime. Run
`scripts/dev/workspace-lab.sh <isolated-agent-root> provenance` for this package
first, or `all` to include storage and Catalog privacy regressions. All builds and
tests run on laba. See the [acceptance audit](../../docs/ai/workspace-provenance-acceptance.md)
for exact snapshots, event digests and remaining issue17 gates.

`RecoveryController` accepts discard-only controls from authenticated channel
actors, with current workspace/resource/task CAS and stable delivery identity.
Migration068 retains immutable actual executor inode bindings, owner decisions,
one-use erasure attempt phases and independent receipts. The installed runtime
polls only its profile's explicit decision queue. Per-copy execution exclusion
precedes PG locks; independently bound Observer absence plus parent sync is
required before reconciliation advances version/fence and retires the copy.
Missing old binding, live tree/lease/executor, foreign/stale owner or source,
changed CAS and invented receipts deny. `erasing` resumes observation only;
another physical attempt requires another owner delivery with previous-decision
CAS. Original unknown effects, source history and allowance stay retained.
See [discard acceptance](../../docs/ai/workspace-owner-discard-acceptance.md).

An unattempted requested intent made stale by owner task/constraints changes may
be superseded only by a distinct explicit current owner delivery with previous-ID
and full current CAS, unchanged resource/copy/source/root/lease proof and NULL
attempt. Current requested intents cannot be superseded merely by another
delivery. Old workers remain denied; old delivery replay returns its own retained
identity/phase/frozen CAS, never the successor's current authority or receipt.
