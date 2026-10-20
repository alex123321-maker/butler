# Notification task context and automatic local memory search

Implemented composition extends the existing Session/Task/Run, Catalog, auxiliary
jobs and pinned EmbeddingGemma2 reader. PostgreSQL remains authoritative; vectors
are derived discovery data, not accepted facts or authority.

## Task context

An authenticated durable initiative task with a mobile batch uses only primary
messages from runs of that task. Ordinary conversations still replay their full
session or verified summary/tail. Session ownership, ordering, lease, transcript
revision and source privacy guards remain unchanged. Current typed task goal,
constraints and mandate are always attached. Recovered current inputs occur once.
The full audit transcript remains stored. Each inbox/search response includes at
most8192 UTF-8 bytes of message text; longer originals remain available through
bounded memory.open. A task continuation reads durable pending decisions.

## Notes and fragments

Broker-dispatched messages.process(remember) atomically commits the immutable
source-dependent note, one stable note fragment and the terminal message decision.
An exact retry does not duplicate publication. An explicitly permitted index
scanner repairs missing fragments for older active mobile notes. Notes remain
source-backed observations, not owner-confirmed Profile facts. Event proposals,
confirmation and cancellation must be represented in their note text and source
quotes; this change does not infer or accept temporal fact metadata.

## Explicit automatic maintenance policy

Owner-only GET/POST /api/v4/memory/index-policy lists/creates the current owner's
policy. POST takes policy_id, exact space_id, max_chunks(1..10000),
max_jobs_per_day(1..96), finite allowance, expires_at(up to366 days) and
quality_proof(the existing reviewed Proof for initial model qualification).
Creation is idempotent only for identical immutable parameters; one enabled policy
per owner is allowed. Disable the old policy before creating a replacement;
old records and job links survive. GET also reports the runtime space_id. Owner-only
POST /api/v4/memory/index-policy-control takes policy_id, expected_revision and
enabled. Disable/re-enable increments revision and fences old jobs.

This explicitly authorizes local maintenance of the owner's active user-scope
fragments and their current original dependencies. It grants no project/global
scope or cloud use. A30-second scanner coalesces changes, then atomically enqueues
an ordinary kind=index memory_aux_job with exact source/ancestor manifest,
independent deadline/allowance and policy-revision projection. No expired run
lease or fabricated owner actor is used. Every actual IPC and publication checks
the current policy, claim, source graph, local identity and shared budgets.
Existing four-claim retry and vector reuse apply. Daily admission is bounded;
failed generations never replace the active pointer. A job is finite even while
its parent maintenance policy is longer-lived. Inactive/expired policy stops new
work and in-flight publication; source privacy still governs older active vectors.

After complete current coverage, the worker marks the generation ready using
an owner_authorized_maintenance receipt and CAS-activates against its pinned
previous pointer. This is mechanical maintenance under an initial reviewed model
policy, not a fabricated per-generation quality experiment. Manual index review
retains its existing reviewed-proof contract. A profile change requires a new
reviewed policy; callers cannot widen immutable allowance/scope snapshots.

## Search

The fixed Broker memory.search control uses the existing HybridReader with local
query admission independently of experimental adaptive retrieval. In composed
ordinary reads, an omitted strategy on page0 selects semantic_hybrid, with at
most four results. Explicit catalog_exact remains lexical and paginated; explicit
semantic_hybrid works without adaptive classification. Event time filtering still
requires the separately composed adaptive control. No inference runs inside the
owning read transaction. Every candidate is reauthorized in the current task's
Catalog scope before delivery. Existing exact/simple/Russian/vector fusion is
reused; reranking stays off. No hybrid candidates falls back to catalog search
with explicit semantic_empty_lexical_fallback diagnostics. Search has a bounded
30-second cold-model allowance; other reads retain five seconds. Query/index
calls share the existing serialized local GPU worker and ordinary budgets.

## Archive expiry and retained evidence

New devices default to360 days (owner configurable1..365). Existing devices retain
their explicit setting until owner control changes it. Selected remember notes
and their exact evidence have no age/count TTL. This does not promote observations
to accepted Profile facts or establish unlimited semantic job resources.

The same remember transaction converts its ordinary raw-body dependency into
memory_retained_mobile_evidence: an immutable typed privacy edge with exact source
ID/revision/hash/access revision, message ID, app, sender, conversation, posted
time, and a credential-masked exact prefix of at most2048 Unicode characters.
Truncation is explicit. Evidence is extracted by trusted Go code from the original
message part, never supplied by a model. memory.open returns retained_evidence
alongside the note; the raw source reference remains historical after expiry.
Corrections/access changes invalidate retained evidence; stale observations are
never presented as current facts. Reported event validity remains separate from
physical retention and owner acceptance.

Automatic cleanup appends archive_expire to the fsynced independent deletion
manifest. It erases the raw body, ordinary derivatives and raw-dependent caches,
while preserving retained notes, evidence, fragments and derived vectors. Reads,
indexing and provider release recheck the exact original metadata under a shared
lock, allowing only the pinned active original or its exact single archive-expiry
tombstone. Explicit delete/revoke follows both ordinary and typed privacy edges
and physically erases retained evidence, facts and vectors. Deleting a note alone
does not erase its original or sibling notes. Manifest reconciliation after a
database restore preserves that distinction and prevents resurrection.

The authorized local scanner backfills older notes before fragment repair, with
the original still readable. Expiry and operator manifest replay also preserve
already-committed remembered decisions from older backups, before raw erasure.
This narrow erasure-only seam validates the exact same owner/user scope, original
revision/hash/access, private source, remembered message binding and single
ordinary dependency; it grants no run/model access and accepts no new fact.
It needs no active indexing policy or fabricated owner actor. Invalid/missing
original proof fails before erasure, requiring repair rather than silent loss.
Housekeeping protects class rows backed by retained evidence from age/count
pruning. Explicit forgetting remains available through owner Catalog deletion.
Migration088 refuses rollback while retained evidence or archive tombstones exist.

Fixed memory arguments are decoded during Broker validation before permanent
effect preparation/dispatch. In particular, messages.search(limit:50) is a
validation failure; the declared maximum is10. Actual lost/dispatched outcomes
still require ordinary reconciliation and are never relabelled as success.

## Acceptance and operations

Run scripts/dev/mobile-memory-tests.sh on laba: isolated PostgreSQL, real Broker,
actual pinned local index and a top1 paraphrase query among unrelated notes.
Targeted regressions cover1000 unrelated batches, current-input deduplication,
automatic activation, unchanged-scan idempotence, widened-policy rejection,
revocation during inference and source deletion. Native fixtures are synthetic,
not claims of broad private-memory quality. Production backlog recovery uses
owner-authenticated task control resume, a fresh input and current authority;
no direct SQL completion or blanket reopening of blocked tasks is permitted.
