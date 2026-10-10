# Memory operator initialization

`memory-admin -initialize-manifest /persistent-private/deletions.log` creates a
new private append-only deletion anchor once, using O_EXCL and file/directory
fsync. It never replaces an existing file or connects to a model. Set the same
readonly operator environment `BUTLER_MEMORY_DELETION_MANIFEST_FILE` in the
orchestrator and Tool Broker; both processes must read it, only orchestration
writes it. Persist the directory independently of PostgreSQL backup rollback.

Missing, broadly readable or truncated anchors block memory access and model/
dispatch/result release. An operator restores the complete newest anchor, then
orchestrator startup reapplies unapplied tombstones before accepting memory.
Never initialize a replacement empty file after loss or a database restore.
Old backup media must also be expired/erased under the owner's retention policy;
reconciliation protects application use, not someone reading a backup offline.
