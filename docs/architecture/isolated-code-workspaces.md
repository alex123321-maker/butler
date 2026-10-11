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
Real PG/filesystem tampering tests use this observer; production Broker provenance,
immutable evidence persistence and runtime composition remain separate work.
