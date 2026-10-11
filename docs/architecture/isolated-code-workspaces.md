# Isolated code workspace boundary

The kernel file storage part of issue #17 is implemented in `internal/workspace`
and migration 060. The detailed API, storage and test contract is its
[README](../../internal/workspace/README.md). Deployment and model access remain
disabled until current owned resource leases, shared budgets, source/provenance
policy and the Broker execution path are composed by the kernel.

Workspace identity is allocated by the kernel and bound immutably to owner,
session, task and task/constraints revisions. Host paths and private lease proof
cannot enter model arguments. Resource identity is exactly `workspace:<ID>`.
The current run/task/goal/mandate and resource lease are checked on the same PG
transaction before workspace observations and again at DB time after waiting.
Separate copies have independent private data/temp trees; writes require both a
monotonic file version and physical content hash. Concurrent approved patches
cannot overwrite one another, including across manager instances.

The mandatory `Guard.ConfirmCreateTx` and `Guard.ConfirmWriteTx` record resource observation and write-lease
consumption with the committed file metadata on the caller's transaction. The
guard takes resource update locks from the first create/write check, avoiding
lock upgrades. Confirmation failure after rename blocks the copy and retains
the uncertain-effect checkpoint; it never reports an invented filesystem undo.

Filesystem and PG commit are distinct. A synced content-free pending marker,
physical mismatch or failed acknowledgement causes honest blockage rather than
automatic replay or invented rollback. Owner reconciliation requires a separately
authenticated privileged control path. A fresh worker lease is insufficient.
This primitive protects prepared code artifacts; it executes no shell/package
code, grants no model capability, copies no environment/secrets and accepts no
fact/profile/permission memory writes. Current programmatic tests cover actual
PG and files under race detection, not arbitrary code sandboxing or delegated
resource-cost experiments. Full #17 closure also needs the separately owned
browser/resource/delegation/kernel integration acceptance.

The typed ResourceGuard adapter now composes actual resources.Store with mandatory
exact action/source admission and independent physical evidence callbacks. Claims
must be committed before creating a namespace or writing bytes. Successful Create
consumes its claim/version; a subsequent Write needs fresh resource admission.
The private actual body digest is computed before the first write guard; a read
digest is obtained from verified physical metadata before release. Actual PG/pool2
tests cover this composition, while the production Broker policy/evidence adapters
and all runtime routes remain disabled until separately verified.

Independent physical confirmation is now provided by the kernel `Observer`,
using a separately opened fixed private root and the caller's transaction. It
returns content-free actual namespace/file observations and checks exact durable
binding, physical hash/length, opened namespace/tree identity, private directory
permissions, empty temporary state and the synced opaque checkpoint. It supplies
no authorization or reconciliation decision. The adapter repeats current
authority after observing and consumes the resource on the same metadata commit.
Real PG/filesystem tampering tests use this observer; production Broker dispatch,
immutable effect evidence and runtime composition remain separate work.

`internal/workspaceexec.Provenance` now supplies current source policy and
immutable cumulative original graph through Catalog task-scoped empty-body DATA.
Private context is prepared before effect/budget/resource/workspace locks from
actual run graph roots and old copy history. Ancestor pins do not become child
body grants. Publication on physical/file/resource confirmation retains source
scope, original parents and observation digest; later runs independently check
that same original graph. Owner actors and auxiliary job proof cannot substitute
for current worker authority. No accepted memory/policy or duplicate body is
created. See [source/privacy composition](../../internal/workspaceexec/README.md).

Migration066 joins transitive Catalog deletion to atomic workspace blockage and
an exact durable erasure queue. A fixed private-root DeletionRuntime removes only
the opaque copy after the owning privacy job verifies committed tombstones and
the deletion anchor. Failure/lost acknowledgement retains pending cleanup;
restart/idempotent erasure never resumes execution. This is privacy cleanup,
not owner reconciliation.

Migration067 and `workspaceexec.DispatchStore` now join exact permanent action
admission to actual source/input/worker/task/shared budget/resource state on one
transaction. Private immutable operation/path/body digest/file CAS and original
input-claim/source bindings precede a bounded one-use runtime ticket. A stale file
snapshot denies before admission commits, retaining grant and budget unspent.
The canonical three DATA contracts never accept host paths, credentials, tickets,
source policy, executor identity or a successful verdict from the model.

The private authenticated workspace gRPC runtime uses the actual Store and
independent Observer. It consumes its start before touching files, publishes
immutable physical evidence with metadata/source context/resource completion,
and releases reads only after current final checks. Runtime output contains only
the opaque namespace and bounded code/snapshot. The Broker Server/router composition
is tested with real PG/files and lost actual-effect acknowledgements. Unknown
outcomes remain stopped at irreversible admission and cannot be replayed or
dispatched through a fresh tool.

Production application composition is now available under operator-only,
default-false `BUTLER_WORKSPACE_ENABLED`. Owning bootstrap checks source/anchor
and intact dispatch/input/history schema before projecting readiness and a fresh
private input claim hash. Broker installs only its canonical contracts after
exact authenticated runtime readiness, rejecting raw workspace registry routes.
The fixed Unix socket or literal loopback target is operator metadata; DNS/proxy/
LAN bearer fallback is unsupported. Workspace/browser admissions retain separate
guards, and unsupported browser resource routes stay out of scoped discovery.

The dedicated `tool-workspace` application owns its fixed private artifact root,
private service token, actual PG/filesystem runtime and scheduled bounded Catalog
privacy cleanup. Independent root identity/permissions, profile/target/contract
hashes, deletion anchor and enabled immutable-history triggers gate readiness.
Compose shares only the private socket and token with Broker; artifact data and
private temporary files remain exclusive to the runtime. Actual application tests
cover loopback and Unix processes and scheduled physical source erasure.
The overlay is explicit and has not been applied to main laba. Unknown writes
remain quarantined; authenticated owner reconciliation and remaining integrated
resource/delegation acceptance are still required for full#17.

Each production workspace Runtime now holds an exclusive Linux directory-inode
`flock` for the lifetime of its fixed private root. A second participating process
cannot open that same root, even under a different socket/profile; independently
provisioned roots remain concurrent. No PID, lock file or shared temporary data
is created. Unsupported platforms/filesystems fail closed. Readiness and runtime
entry bind the held directory inode to the current private operator path, and
the Store admission callback repeats identity checks after authority/resource
waits. Privacy erasure validates root identity around physical work. Private
mounts, os.Root containment and OS/runtime isolation remain required.

Shutdown excludes new execution/readiness/privacy entries, drains already
entered work, closes its file/Observer/privacy handles and only then releases
root ownership. SIGKILL releases the operating-system lock, while PostgreSQL
claims, inflight markers, effect history and unknown outcomes remain unchanged.
This lifetime primitive supplies neither execution authority nor evidence of
effect absence. Authenticated owner reconciliation must still independently
prove old-executor quiescence, exact evidence, authority and lease expiry; a
successful restart alone cannot reconcile a resource or replay an operation.
