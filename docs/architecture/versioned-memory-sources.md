# Versioned memory sources and privacy

PostgreSQL remains authoritative for Transcript, Working, Episodic and Profile
Memory. Migrations 046–049 add a catalog of stable source IDs, immutable content
revisions, reported author/event time, extraction time, scope, inherited private
provenance, current access revision and active/quarantined/revoked/deleted status.
The original message or document and its fragment locator remain separate from
retrieval chunks and derived vectors. Current tasks, constraints, owner goals,
mandates and approvals continue to use their typed operational stores.

## Decisions and current access

An extraction is a candidate even at confidence 1. Owner acceptance locks the
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

The memory page captures the current in-page owner token and a proof epoch for
each operation. Changing or clearing the token, or leaving the page, clears
private views and editable fields. A response from an older epoch cannot publish
source text, a citation, a receipt or a download, nor start a follow-up request
under a replacement token. A `source_id` evidence link opens that exact source
only after the owner explicitly loads the authenticated page.

Live retrieval checks the current run lease/task/assigned goal authority, exact
source revision/hash/access and scope. Explicit source grants bind a task revision
and purpose. An auxiliary worker instead supplies its own current durable job
claim and full source manifest; it cannot borrow a completed run's lease. Catalog
rows lock in sorted order before chunks/vectors, including every original parent.
Publication uses the same transaction and fresh proof after local inference.
Source and extraction-deduplication locks may wait past a lease or job deadline.
Catalog access repeats time-sensitive run/task/delegation or independent job
proof after those waits, including before returning an exact retry's candidate.
These checks use already-held authority/source locks in the same transaction;
they neither borrow another pool connection nor reacquire authority after new
source locks in a separate transaction. Confidence never makes a claim accepted.

Query locality and external release are separate decisions. Assigned privacy
other than cloud_allowed preserves permitted local search; only the exact pinned
internal backend plus matching current LiveAdmission may receive its query or
reranker snippets. PostgreSQL exact/full-text lookup can still operate without
that inference backend, with explicit degraded diagnostics. It never falls back
to an unproven legacy embedding provider. The task/assignment/privacy stamp and
current pinned sources are checked again after retrieval, and ordinary provider
release admission still applies to the query, snippets and complete request.

Preparing model context pins transcript source revision and deletion watermark.
Retrieval additionally pins every source ancestor. Each provider release and its
event stream rechecks current source proof; a 250 ms supervisor cancels idle calls.
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
Owner task controls preserve only the exact already-empty working state with its
same-owner deletion marker. Changing or adding working content still invokes the
normal run source fence. Stop/amend do not obtain a privacy-redaction exception,
and a new amendment lease cannot read the deleted original.
The idempotent cache worker persists a 500-key cursor per deletion and removes only
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
`BUTLER_MEMORY_DELETION_MANIFEST_FILE`. Provision directory 0700 and file 0600 for
container UID 10001 before rollout. Initialization requires a new nonexistent file;
never generate an empty replacement after loss or restore. Keep the full newest
anchor outside PG backup rollback and retain private independent copies. Restore
that anchor first, restore PG second, then run reconciliation before access.
Missing setup is logged generically and fails closed even if HTTP health responds.

The deployment retention policy is at most 30 days for content-bearing PG backups
unless the owner explicitly defines a stricter period. Operator backup rotation
must erase expired copies; Butler cannot manage unknown offline media. Restoration
of any retained backup requires the newest independent anchor and reconciliation
before access. Tombstones/anchor copies are retained as long as any content backup
or source derivative may exist; do not expire them with the PG content backup.
Application replay prevents reappearance through Butler. Old backup media and
already exported/delivered external copies require the owner's separate retention
and erasure procedure; no application can retract an offline copy it does not own.

## Agent candidate proposals

Agent reads use separately gated fixed Broker `memory.search`, `memory.open` and
`memory.neighbors` contracts. Search returns bounded DATA citations; exact
source/revision/access/hash and catalog fragment keys are required for opening.
Each actual dispatch reconstructs current input/lease/task/goal/mandate proof;
catalog authorization pins complete ancestors. The returned ID never grants
future access. Correction/deletion/access changes deny old handles and ordinary
provider release checks protect all returned evidence. See
[read contract](../../internal/memory/readcontrol/README.md) for bounds, distinct
statuses and the initial PostgreSQL full-text strategy.

Default-false operator composition `BUTLER_MEMORY_CANDIDATE_ENABLED` joins the
fixed local `memory.propose_candidate` contract to the catalog candidate queue.
Broker requires current owning readiness/task policy, exact preparation/authorization,
permanent dispatch and shared budget reservation. Its private fixed endpoint is
`/internal/memory-candidate`; model arguments cannot set owner/scope, confidence,
acceptance, job/lease/input authority or retry identity.

Orchestrator reconstructs the actual Session lease and the exact input claim
pinned in that grant. It checks the tool-dispatch checkpoint, current task/goal/
mandate/constraints and complete source graph in the caller transaction before
and after publication waits. A source must already be pinned by the run and have
current `memory_extract` access; the pin alone does not prove that the model read
every ancestor. The catalog derives owner/scope from the original source, assigns
confidence0 and deduplicates with the actual effect ID. Recognized credential
keys/patterns deny publication; source sensitivity remains independently checked.

Proposals append DATA candidates; acceptance separately requires an authenticated
owner and exact head CAS. Source correction/deletion denies stale publication and
removes covered text normally. Context-only/read-only child contracts receive no
new scope, tool or extraction rights from this flag. Lost RPC acknowledgement
does not settle an unknown Broker effect or authorize replay. This composition
does not run a model, enable legacy cloud extraction or establish semantic quality.

## Owned auxiliary work

`memory_aux_jobs` records immutable owner, purpose, original run/task and assignment,
task/constraint versions, complete source hashes/access, finite allowance/deadline,
and current claim ID/epoch. Current job snapshots must match every saved policy
field, complete canonical source fingerprint and claim deadline before data access
or budget initialization. Expiry/takeover denies old reads, inference, publication
and finish. Every actual attempt reserves job/root-task/mandate/owner-day allowance,
then rechecks the database clock after budget waits. Admission returns a bounded
context which the actual IPC must use; late callbacks cannot publish.

The original local auxiliary admission accepts only the operator-pinned internal
service identity/endpoint. `local_no_billing` is a price declaration, not proof of
locality. Secret inputs fail closed. Its task jobs retain original assignment and
require an active current task; a completed task needs a separate explicit owner
policy, not dropped task IDs or a revived lease. Legacy model extraction and
summary workers remain disabled. Owned local indexing is separately opt-in and
uses current job-owned authority for all reads/inference/publication; live query
uses current run ownership. See the indexing Runtime README for exact composition.

Migration071 extends this queue with finite independent owner policies and the
configured generative transport. Exact current source/task/mandate/privacy proof,
per-attempt shared budget reservations and atomic catalog DATA publication are
required. Completed root tasks keep their current revision and identity; local-only
data waits without cloud fallback. Production consumers remain separately gated.
See [background memory jobs](background-memory-jobs.md) for triggers, controls,
failure accounting, lock order and rollback.

## Structured ingestion and owner UI

The owner memory page uses only versioned private APIs. It displays source/event
and ingestion dates, immutable revisions, candidates/confirmed/superseded history,
original assets, failure diagnostics and fragment/neighbour context. Corrections,
acceptance, grants and deletion check current versions; no vector score or source
recency silently resolves conflicts. The token remains only in page memory.

Owner-selected Go, Markdown/text, CSV/TSV, messages and DOCX use bounded standard
Go parsers. The optional dedicated Poppler/Tesseract rus+eng sandbox handles real
PDF text and scanned OCR. Stable keys preserve unchanged entities/citation-only
renames; original bytes and locators remain available until deletion. Tables
preserve header/row context; OCR keeps method/uncertainty. No Python dependency
was added. See [structured ingestion](../../internal/memory/ingestion/README.md)
and [parser isolation](../../apps/memory-parser/README.md) for format/size limits.

## Verification

Every new consumer must extend the [mandatory lifecycle security gate](../ai/memory-lifecycle-acceptance.md)
with its own applicable publication/read/provider/retry/deletion/restore evidence.
Retained model or document instructions, quoted owner policy and even a correct
historical success stay DATA; they cannot grant Tool Broker authority or widen a
current task. Selective derivative deletion must preserve independent valid
sources and current typed owner permissions. Existing corpus tests use synthetic
markers only and do not certify model quality or future consumers.

Actual PostgreSQL tests use isolated schemas and MaxConns2. Catalog/job/API,
idle-provider cancellation, Controller/Coordinator boundaries, immutable privacy,
backup replay, wrong history, cache restart and stale publication run under race.
The acceptance cases also prove candidate supersession CAS, foreign-owner denial,
parent correction, forged job manifests/allowance/deadlines and terminal-run grant
denial. Structured upload tests prove Russian tables/messages/long Go entities,
duplicate/rename/single-function change, original access/revoke and corrupt-file
isolation. Owned-runtime tests prove atomic admission/projection rollback, bounded
retry/restart without repeated prefix inference, kind-specific claims, pool2 and
delete during model callback. Actual PDF text/scanned OCR runs use the fixed native
image; mocked PageAdapter tests only cover selection/error logic. Browser fixtures
exercise owner proof, supersession history, correction/export/delete and upload
with original context. Retrieval/index quality is independently measured; none
of these checks establishes truth of extracted claims or new action permission.
