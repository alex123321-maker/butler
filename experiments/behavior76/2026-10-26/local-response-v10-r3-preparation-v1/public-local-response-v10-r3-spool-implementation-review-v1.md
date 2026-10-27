# Static implementation review — private spool, unfrozen v10-r3

Status: two concrete source-contract gaps remain. This is not a formal freeze,
authorship, test result or native admission. Parent narrow tests are running;
checker performed only source/filesystem reads and wrote this review.

The implemented qualified path has the intended order: final private flush,
exclusive destination copy, Sync/Close, destination read-back size/hash, removal
only of enumerated verified duplicates, then sealed=true. The unchanged reader
format still applies. snapshotLocked also avoids the previous unlock/resnapshot
gap. A failed partial retirement preserves destination originals and the private
identity, and cannot qualify the header. No recursive deletion, timeout increase,
reader/scorer/criterion or native-response exemption was introduced in this seam.

1. **Failed closeout does not close recording.** unsealedStreamEvidence sets the
   shared GLOBAL flag and sealed=false, publishes current segments and writes an
   exact private unsealed header. appendStreamLocked checks sealed/observerFailed
   but not GLOBAL or a separate terminal-recording flag. A callback arriving after
   that snapshot can therefore append more events or create a private segment
   which the frozen private header/public inventory does not name. snapshotLocked
   makes one snapshot atomic but does not close the writer. Close recording under
   the same mutex independently of the qualified sealed flag; a later callback
   must retain GLOBAL/drop/fault semantics without silently extending the frozen
   prefix. This does not authorize suppressing an earlier in-flight failure.

2. **The actual spool filesystem is not witnessed.** The new constructor uses
   trusted Linux /tmp and unique0700/0600 files, which is good, but the current
   StorageProfile test asserts only pathname and permissions. Existing resource
   admission measures the PGDATA filesystem. /tmp can be separately mounted; a
   Linux pathname and runtime.GOOS do not prove this spool is on the declared ext4
   device. Retain a read-only actual filesystem/device witness for the created
   spool in the resource/manifest evidence and enforce the declared placement,
   with GLOBAL failure and no fallback or capacity increase if it differs.

The original802ms case is still an authority-observation deadline. Its sole
underlying cause remains unproven. Even corrected local placement leaves
synchronous compression and Sync in pgx callbacks, so this code cannot claim that
all future750ms failures have been eliminated. Final source review and the
existing completed Public61/checker298/vet, source/scorer freeze, fresh controls,
setup/composition and one-use admission remain pending.

No fault test was authored, retried or executed. No Go, PG, auth or native call
was performed by this checker. Closed r2 original files/results remain unchanged.
