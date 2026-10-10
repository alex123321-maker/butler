# Versioned memory sources and privacy

PostgreSQL remains authoritative for Transcript, Working, Episodic and Profile
Memory. Migrations046–048 add a catalog of stable source IDs, immutable content
revisions, reported author/event time, extraction time, scope, inherited private
provenance, current access revision and active/quarantined/revoked/deleted status.
The original message or document and its fragment locator remain separate from
retrieval chunks and derived vectors. Current tasks, constraints, owner goals,
mandates and approvals continue to use their typed operational stores.

## Decisions and current access

An extraction is a candidate even at confidence1. Owner acceptance locks the
scope/key head and names its current accepted claim. A conflicting proposal cannot
replace a preference because its document was uploaded later or ranked higher.
Explicit supersession preserves the old historical claim and original sources.
Reported child conclusions remain data and retain the original ancestor graph.
Owner correction appends a source revision; derivatives pinned to an older parent
stop being usable immediately. Legacy memory is quarantined, not automatically
converted into verified facts. An owner review retains candidate confirmation.
Unattributable legacy global rows stay quarantined until an operator establishes
their real owner through a reviewed migration; the UI cannot claim them.

Authenticated owner controls, including reads and streaming NDJSON export, use
`/api/v4/memory`. The request cannot set owner identity, source authority or parent
links. Export contains all revisions and claims without the inspection-list cap;
an interrupted export ends with `export_incomplete`. An owner-authorized export
already in progress is an external copy and cannot be recalled by later deletion.
Existing scope reads also require owner proof; old unversioned mutation APIs deny.
Tokens use the owner-control configuration and never enter model or memory data.

Live retrieval checks the current run lease/task/assigned goal authority, exact
source revision/hash/access and scope. Explicit source grants bind a task revision
and purpose. An auxiliary worker instead supplies its own current durable job
claim and full source manifest; it cannot borrow a completed run's lease. Catalog
rows lock in sorted order before chunks/vectors, including every original parent.
Publication uses the same transaction and fresh proof after local inference.

Preparing model context pins transcript source revision and deletion watermark.
Retrieval additionally pins every source ancestor. Each provider release and its
event stream rechecks current source proof; a250ms supervisor cancels idle calls.
Broker grant claim and atomic final-result publication check the same pins inside
their existing transaction, with no extra pool connection. Completed, cancelled,
failed or finalizing runs cannot claim a fresh external dispatch. Earlier spent
dispatches retain their immutable effect observations.

## Owner deletion and recovery

The typed owner deletion path recursively removes derived class rows/vectors,
clears original revision/claim text and scope grants, invalidates covered summaries,
and redacts linked transcript, artifacts, final answers and notification payloads.
IDs, original hashes, verdicts and effect evidence remain unchanged. Sending
notifications become `delivery_unknown`; deletion never claims external undo.
Privacy-redacted immutable results cannot subsequently regain content.

Legacy derivatives without complete edges are purged conservatively for that
owner. Task goal/constraints/mandates remain intact, while task scratch is cleared
with a bumped working revision and content-free deletion reason. Old prepared
contexts cannot restore durable or Redis working data. Every transcript head and
global deletion watermark changes, so late summaries cannot republish old text.
The idempotent cache worker persists a500-key cursor per deletion and removes only
the owner's known session/run working keys. A Redis failure leaves cleanup pending
for restart; current reads already deny the stale data. No global cache flush runs.

Deletion uses sessions→sorted tasks→transcript heads→sorted source graph→derived
rows→watermark. Normal release uses ownership→session/run/task→goal authority→heads
and sorted sources before derived writes. Deletion holds no goal/mandate locks.
Source publication must not acquire session/task locks after source locks.

A private content-free append-only manifest is fsynced before the deletion PG
commit. It contains opaque source/deletion IDs, hashed owner, action, order and
time, never text or URLs. PostgreSQL's applied watermark **and each tombstone's
identity/action** must match the manifest. A failed transaction can leave an extra
tombstone in the file; replay intentionally applies that deletion. Missing,
truncated, broadly readable or mismatched files deny memory/model/dispatch/result
release. Orchestrator startup replays pending deletions before memory becomes usable.

Initialize once with `memory-admin -initialize-manifest /private/deletions.log`.
Compose mounts the `memory-deletion-anchor` volume read/write in Orchestrator and
read-only in Broker at `/var/lib/butler-memory`; set both processes to the same
`BUTLER_MEMORY_DELETION_MANIFEST_FILE`. Provision directory0700 and file0600 for
container UID10001 before rollout. Initialization requires a new nonexistent file;
never generate an empty replacement after loss or restore. Keep the full newest
anchor outside PG backup rollback and retain private independent copies. Restore
that anchor first, restore PG second, then run reconciliation before access.
Missing setup is logged generically and fails closed even if HTTP health responds.

Application replay prevents reappearance through Butler. Old backup media and
already exported/delivered external copies require the owner's separate retention
and erasure procedure; no application can retract an offline copy it does not own.

## Owned auxiliary work

`memory_aux_jobs` records immutable owner, purpose, original run/task and assignment,
task/constraint versions, complete source hashes/access, finite allowance/deadline,
and current claim ID/epoch. Current job snapshots must match every saved policy
field, complete canonical source fingerprint and claim deadline before data access
or budget initialization. Expiry/takeover denies old reads, inference, publication
and finish. Every actual attempt reserves job/root-task/mandate/owner-day allowance,
then rechecks the database clock after budget waits. Admission returns a bounded
context which the actual IPC must use; late callbacks cannot publish.

Only the operator-pinned internal local model service identity/endpoint can be
used. `local_no_billing` is a price declaration, not proof of locality. Secret or
cloud auxiliary processing fails closed. Task jobs retain original assignment and
require an active current task; a completed task needs a separate explicit owner
policy, not dropped task IDs or a revived lease. Production legacy extraction,
summary and embedding workers remain disabled pending adapter/job scheduling;
this catalog/privacy implementation alone is not full structured ingestion.

## Verification and remaining integration

Actual PostgreSQL tests use isolated schemas and MaxConns2. Catalog/job/API,
idle-provider cancellation, Controller/Coordinator boundaries, immutable privacy,
backup replay, wrong history, cache restart and stale publication run under race.
The acceptance cases also prove candidate supersession CAS, foreign-owner denial,
parent correction, forged job manifests/allowance/deadlines and terminal-run grant
denial. Structured Go parsers/upload lifecycle, owner UI and production semantic
job adapters remain separate follow-up work before full memory issue acceptance.
