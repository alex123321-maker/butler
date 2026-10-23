# Versioned memory sources and privacy

Android [notification ingress](android-companion.md) atomically publishes private
original sources with content-free receipts. Standing owner-configured mobile
automation activates originals without per-message review; otherwise originals
remain quarantined. Normal initiative/goal/mandate/budget admission processes
20 new messages or an hour of pending messages. Notes and reversible noise
rules are source-dependent DATA derivatives, not owner-confirmed facts.
Deletion and retention use the same external manifest; redelivery retains the
receipt and cannot rehydrate source text.

Migration088 distinguishes archive_expire from owner delete/revoke. Selected
mobile observations retain immutable bounded exact evidence through a typed
privacy edge, independently of raw archive expiry. Explicit forgetting still
traverses that edge, purges evidence and descendants, and replays after restore.
All Catalog/pinned model-job release guards check its current original metadata.
See [retained mobile evidence](mobile-memory-search.md#archive-expiry-and-retained-evidence).

Migration079 extends source/run privacy to primary Transcript tool caches and
copied approval payloads. An irreversible content-free run marker is committed
with erasure; late tool publication and payload rewrites cannot resurrect it.
Exact effects, action/receipt hashes, tool outcomes, attempts and usage survive.
Unrelated runs are retained. Previously withdrawn source pins and redacted
messages receive the same cache erasure during upgrade without changing external
anchor numbering. See the [Catalog contract](../../internal/memory/catalog/README.md).

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

Catalog fragment publication limits titles to2048 bytes and rejects recognized
credentials in exact fragment keys. Agent reads also suppress old unsafe keys
with an explicit incomplete-result flag and deny requests carrying such keys;
masking never creates a replacement handle. Recognized title/text credentials
are masked before clipping, with original Unicode positions and redaction flags.

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

The #85 generative candidate consumer preserves exact source quotation/rune spans,
revision/hash, unknown time and original scope. Its claims remain candidates and
reuse existing owner acceptance/correction/revoke controls. Migration073 adds a
trusted projection class: outcome episodes become Episodic Memory only on owner
acceptance; preferences/facts become Profile Memory. An accepted exact episode may
have no vector until separately admitted indexing. See the
[candidate contract and release gate](../../internal/memory/candidates/README.md).

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
# Owner Web browse projection (2026-10-21)

`GET /api/v4/memory/access` checks the existing owner bearer without reading
catalog data and returns only `{authenticated:true}` with `no-store`. It does not
issue a session token or grant any action. Catalog readiness cannot prevent the
owner from connecting to the other Web modules; every later request authenticates
independently.

`GET /api/v4/memory/browse?limit=6&offset=0` lists active source heads ordered by
creation time and source ID descending. Correcting an older source does not move
it to the front. Limits are 1–20, offset 0–1000; lookahead reports `has_more`.
Selected sources and dependencies pass current `AccessTx`/restore checks before
preview publication. A stale or denied graph fails closed.

`POST /api/v4/memory/search` accepts `{query,limit,offset,strategy?}`. Omitted or
`text` strategy uses PostgreSQL
Russian text search/ranking on active current titles/bodies. Query size is bounded
to 2048 UTF-8 bytes. The response explicitly reports `mode: text`.
Empty queries return recent records. Optional `hybrid` strategy has independent
owner admission described below; no run lease is fabricated.

Both projections authenticate before parsing, return `Cache-Control: no-store`,
bound title/body previews to 200/512 Unicode characters, and mask credentials
before slicing. Secret-classified heads are excluded by the existing access policy. Previews are not
fact verdicts, task grants or substitutes for current exact-source inspection.

`POST /api/v4/memory/notes` accepts only `{title,body}`. Owner and user scope derive
from the authenticated actor. Source identity derives from required delivery ID;
an exact retry returns the same source. A changed body cannot overwrite its
revision silently. Correction/deletion retain existing version-pinned endpoints.

## Owner Web memory explorer (2026-10-23)

The continuous home surface opens a read-only source explorer rather than making
source selection an edit form. It reuses browse/search, source inspection and the
catalog claim/decision APIs; no new memory store, inference admission or service
is introduced. A source's active status means saved/accessible, not owner-confirmed.
Only claim records with matching source ID/revision display fact decisions.
Confirmation includes the observed `expected_active_id` for the exact scope/key;
conflicts or unknown outcomes require a fresh read before a new decision.

List and graph share the query and selected source. Graph edges represent exact
stored `origin.parent_refs`, including historical provenance, not semantic truth
or grants. Only loaded source endpoints are drawn; external references require
a new authenticated inspection. Counts and graph coverage are bounded by loaded
pages, with explicit continuation and the existing offset limit. There are no
fabricated semantic matches, inferred links or full-corpus count claims.

Inline source/history disclosures show supplied text and immutable revisions. Secret,
redacted or non-active source bodies are withheld from this simplified viewer;
historical secret-class revisions are also hidden. Owner change aborts requests,
destroys private panels and clears selections, recent queries and navigation.
Selection and catalog-load generations fence late results. The last five successful
queries stay only in the current page's owner-scoped memory; they are explicitly
clearable and never persisted in browser storage. The bounded navigation trail
holds source IDs, not cached source bodies, and re-inspects on every visit.
There are no temporary pins. The mobile reader replaces the list in the same
dialog; back restores list position, while source/history expand inline. Owner
claim rejection uses the same existing decision CAS as confirmation; unknown
outcomes require a fresh read. Management preserves the exact inspected source
ID in the existing authenticated route. Clipboard export occurs only on an
explicit copy action. A
reversible pause switch is deliberately absent: source revocation currently purges
content and must not be presented as temporary exclusion from retrieval.

## Owner Web hybrid search (2026-10-23)

Typing remains lexical. Enter or the search button explicitly requests
`strategy: hybrid`, a nonempty query and offset0. Unknown strategies and hybrid
continuation offsets are rejected before model access. Existing bearer-owner
authentication, delivery identity, strict JSON decoding and no-store apply.
The owner cannot supply a model, endpoint, scope, generation or run identity.

When the existing owned semantic runtime is configured, one pinned local
EmbeddingGemma2 query vector searches at most12 active user/session generations
belonging to that owner and matching the exact configured index space. The user
scope is preferred, followed by newest session generations. Extra scope coverage
is explicitly reported as limited. Project/task/global sources and any sources
without active indexed fragments remain in the lexical path. No index or job is
created by a search, and no cloud fallback or reranker is permitted.

`budget.AdmitOwnerQuery` authenticates the web actor, validates exact local
pricing/target, then uses the existing PostgreSQL reservation ledger. Each actual
query has at most1 turn, 2048 input bytes, 4096 conservative token units and30s.
A fixed12-attempt UTC-minute scope is shared across tabs/processes, including when
the optional owner/day cap is disabled. Normal owner/day reservation also applies
when enabled. Minute scope expiry includes30s for requests admitted near its end;
the request deadline never extends. These are conservative reservations, not
measured model usage. No finished run, task mandate or job claim is borrowed.
Only query text is released; restore proof is checked before and after admission
and after inference. HTTP cancellation propagates to local RPC.

Vector candidates retain exact chunk timestamps and index identities. Existing
semantic guards reject changed/deleted/secret sources and ancestry. Catalog
resolves candidates back to current owner source heads, checks source access and
locks current fragments before returning masked bounded previews. At most four
semantic fragments become source results. Text lookup runs after inference;
reciprocal-rank fusion deduplicates sources with match=text/semantic/both.
No numeric semantic similarity is presented as confidence or factual truth.

The initial response preserves all lexical page items plus at most four semantic
items (at most24 with limit20), mode=hybrid only when semantic items were returned,
and content-free diagnostics. `next_offset` counts consumed lexical rows, not
merged visible items. Later pages are lexical only; the client deduplicates them
and retains the initial semantic mode/coverage. No query-vector/body cache is
stored in the browser. Fresh input aborts and invalidates earlier requests.

The semantic subrequest has25s inside the existing30s HTTP bound. Missing index,
budget exhaustion or model/access failure discards semantic candidates and
falls back to a fresh guarded text read; if that read fails, the request fails.
Backend error strings are never serialized as diagnostics. Empty vector results
and partial coverage are distinguishable from a fully semantic result.
