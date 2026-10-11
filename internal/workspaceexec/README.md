# Workspace execution composition

`Provenance` joins actual workspace metadata to the owning memory Catalog on the
same PostgreSQL pool/transaction. It grants no tool, runtime, budget or credential
permission. Production Broker dispatch and runtime/bootstrap composition are
still pending; the current actual action admission in tests is an explicit
fixture, while source policy, storage, physical observation and resource leases
are the real components.

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
