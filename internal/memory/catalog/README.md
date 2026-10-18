# Memory source catalog

The Android ingress uses `PutMobileObservationTx` to create immutable private
quarantined sources in the authenticated receipt transaction. Background device
retention uses `DeleteExpiredMobileObservation`: it rechecks expiry under a
current policy lock and shares the normal external-anchor erasure path without
manufacturing an owner actor. Unexpired/non-mobile sources are denied. See the
[adapter contract](../../mobile/README.md) and
[acceptance report](../../../docs/testing/issue-57-android.md).

Migration079 closes cached tool/approval payloads under exact affected run pins.
Delete/revoke and external-anchor reconciliation erase their args/results/errors
and copied approval prose while preserving tool identity/status, confirmed or
unknown effects, digests and charged attempts. A content-free irreversible run
marker prevents late payload resurrection. Unrelated runs remain intact. The
migration also sweeps already withdrawn/redacted runs of the same recorded owner;
it neither renumbers the external deletion anchor nor reopens an old window.

Actual read permission remains current after a historical lookup. A tool result
is a primary Transcript cache, not a second permission to keep deleted source
bytes. Existing release guards also protect late tool Transcript inserts.

Ordinary token-only standalone source observations hold the complete current
lease/task/source/temporal proof through bounded rollback. They select no new
source and need no durable observer commit; the200ms deadline remains intact.
Compound owner/checker contexts and owning dispatch/publication transactions keep
their existing commit semantics. Fixed phase/pool diagnostics contain no source
text, SQL parameters, raw errors or credential material. See the
[audit and retained failure](../../../docs/testing/issue-88-sourceguard-diagnosis.md).

[Temporal memory](../../../docs/architecture/memory-temporal.md) versions owner
validity interpretation separately from source EventTime/ingestion and existing
head decisions. Typed accepted-fact reads cap a whole page at64, count unknown
validity, and pin current head/source/metadata/freshness. Derived and ordinary
run pins survive restart and deny late publication after metadata-only changes.
Logical expiry labels or excludes current retrieval; it does not erase history,
supersede an owner preference or create a privacy tombstone.

An owner can review an actual already budgeted Broker `http.request` read using
`SetTemporalFromBrokerRead`. The exact successful read receipt, original prepared
fact pin, dispatch, normalized saved result and resource hash must match. The
verification instant is the original observed time, so replay cannot refill TTL.
This method never performs HTTP and no receipt automatically asserts semantic
truth. Failed/unavailable reads retain unknown/stale verification. Private owner
API controls and normal context enrichment remain default-off and unqualified.

The optional [candidate control](../candidatecontrol/README.md) publishes agent
proposals using current run/input/dispatch and source proof. Its trusted effect
deduplication appends neutral confidence0 candidates. Concurrent workers cannot
write accepted preferences; independent owner decisions require the exact current
head. The legacy cloud extraction pipeline remains disabled.

Delegated runs select only their exact contract/source roots and their own
session/task bodies. Original dependency ancestors remain privacy pins rather
than additional selectable context. Catalog owns same-transaction child grants
and rechecks the current parent grant chain at reads, dispatch and publication;
copied child grants cannot retain revoked parent permissions.

`GrantDelegatedCandidateTx` is the narrow technical converter-return bridge. It
requires a current owned parent worker and exact accepted result-ready child,
selected roots, trusted return checker and complete current privacy graph. Its
savepoint grants only the child candidate source; final/report/session access is
excluded. Normal access is repeated after insertion and errors roll back the
grant even if the surrounding transaction commits.

`CreateDerivedTx` creates a fresh immutable `model_candidate` DATA source from
exact current parents under an owned run/job or authenticated owner proof. It
derives owner and sensitivity, anchors scope to the first trusted requested
parent, refuses secrets, and shares the caller's
result CAS transaction. It neither manufactures an owner actor nor accepts a
fact. Parent deletion/restore reconciliation removes the derived text. Voice and
skill adapters may use this hook; model content remains data regardless of labels.
Different scopes require the same proven owner and independent current access
to every parent and ancestor. Child scope never grants access to a parent; all
dependency edges survive and are rechecked on subsequent reads and publication.

PostgreSQL owns stable source identities, immutable revisions, event/extraction
times, reported author, origin, scope/access revision and current source status.
Profile/episodic/chunk stores remain the four-class memory model's class stores.
The catalog links their object IDs to source revisions; vectors are derived.

Fragment publication limits titles to2048 bytes and rejects recognized
credentials in exact fragment keys. A failed publication leaves the original
ingestion asset available for owner review without publishing unsafe handles.
Agent reads independently suppress old unsafe keys and mark incomplete results;
they never turn a masked key into an apparently usable fragment reference.

Owner decisions use `owneridentity.Actor`. Extracted claims are candidates even
with high confidence. `Decide` locks an exact scope/key head and requires the
current accepted claim ID; only that owner decision supersedes a prior value.
Documents, model assumptions and subagent reports never change task authority.
Provenance/dependencies retain original sources and sensitivity.

`AccessTx` checks a live run ownership token or independent owned job proof,
current task/goal assignment, exact permitted scope, source revision/hash/access
and parent currentness. Source locks precede chunk/vector locks.
`CheckObjectsTx` derives the source graph from PostgreSQL object IDs, not citation
JSON. It uses the caller transaction and does not need a second pool connection.

Deletion locks sorted affected sources, removes recursively derived class rows,
redacts source/claim text, invalidates covered summaries, revokes scope grants and
retains content-free tombstones. A typed private file manifest is fsynced before
the PG commit. `CheckRestore` denies reads/indexing when PG's applied watermark
differs from the manifest; `Reconcile` reapplies deletion before restore is usable.
The operator initializes the manifest once, persists it outside ordinary PG
backup rollback, and retains its complete history. Missing/truncated/wrong files
deny access. A failed PG commit can cause extra deletion on replay; this is safer
than resurrecting text. Backup files themselves still require private retention
and erasure policy. The manifest contains opaque source IDs, hashed owner identity,
deletion IDs/order/time and action only, never source text/URLs.

Production adapters and authenticated `/api/v4/memory` use the catalog. Transcript
preparation/model release, Broker grant claim and atomic results pin current
sources; deletion has a one-way typed privacy path retaining evidence hashes and
verdicts. Scoped cache cleanup persists progress and task scratch revisions prevent
late republishing. Full manifest identity is checked on restore, not just its count.
Deletion also redacts source-pinned run metadata and accepted-input checkpoints
before a first message exists. Restore replay erases those copies again while
retaining run/source/effect identities; a queued or late prepared release is denied.

Tests use `BUTLER_TEST_MEMORY_POSTGRES_URL`, an isolated `_test` DB, private schemas,
pgvector in public and pool size two. Actual race tests cover privacy/backup replay,
cache restart, stale provider/final/dispatch writes and owner correction/export.
Structured ingestion, private owner UI and opt-in semantic job/live adapters use
this same catalog. Inspect retains original revisions/fragments/adjacent context;
export includes original assets without list caps. See [the contract](../../../docs/architecture/versioned-memory-sources.md)
for the required private persistent anchor setup before deployment.

The [mandatory lifecycle gate](../../../docs/ai/memory-lifecycle-acceptance.md)
adds the shared eight-case attack corpus, selective derivative removal, corrected
ancestor fencing and independent valid-memory preservation after restart/actual
backup replay. New consumers must extend the same applicable boundaries.

`ProposeTx` supports trusted structured extraction checkpoints and exact retry
keys in the caller transaction. It rechecks current owner/job/source access,
publishes only candidates and rejects a changed body under an existing key.
Source and deduplication row waits cannot extend worker authority: current run
lease/state, task/assignment and delegated deadlines, or the independent job
claim/deadline, are checked again in the caller transaction after the wait.
Even an exact retry cannot return its original candidate to an expired worker.
`DedupKey` is not an HTTP/model field; ordinary owner manual proposals keep their
random identity. Migration043 preserves only opaque IDs/hashes/times separately
from source/claim text, including after catalog deletion.

`VerifiedResultParents` is a content-free historical evidence bridge for reviewed
skills. It binds the actual assistant message to its run, checks restore identity
and every original run/source edge, and returns the exact final-message root.
It grants no publication authority. `CreateDerivedTx` requires a fresh current
owner/run/job proof for all parents; deletion of any original dependency erases
the procedure body while content-free skill audit remains in migration051.

`ReadDelegatedRun` releases only the current worker's selected immutable contract
and source root bodies, bounded to 64KiB. Canonical original dependencies remain
privacy references, not extra selectable bodies. It denies inherited owner/job
proofs, stopped/expired workers and current parent grant or source revocation.
`CheckDelegationProposalSourcesTx` also checks the parent's current purpose/scope
grants when an authenticated owner independently has broader Catalog body access.

`CreateTaskDerivedTx` is narrower: only a live owned worker may create fresh DATA
in its current pinned task scope. Owner and auxiliary-job proofs cannot select
that scope. The complete selected parent graph remains current and deletion-linked;
the method uses a savepoint on the caller transaction and grants no parent-task
read access. Reviewed developer delegation can use it for a child candidate without
making the original parent/project scope ambient child context.

Migration079 down is allowed only before any irreversible run payload marker
exists. Once deletion has fenced a run, schema downgrade refuses to remove its
privacy fence. Roll back optional consumers/images while retaining migrations
and the independently held deletion anchor; never drop live deletion metadata.

Migration080 extends episodic provenance to exact stopped-input metadata heads.
`StoppedAttemptParents` retains primary original messages and the complete used
source graph without requiring or inventing a final message. Full access/release
locks a shared per-input SQL fence; material input/run writers take its exclusive
side before commit. Supervision reads content-free head metadata, never the large
checkpoint. Input deletion/reinsertion cannot revive an older observation.
Populated stopped-attempt provenance refuses schema rollback.

Migration081 adds distinct reconciliation supplements to those observations.
The old terminal witness remains stale after ordinary owner Resume; current
reads retain its metadata/access requirement alongside the fresh observation's
primary privacy graph. Source release locks original and lookup tools, input
heads and exact control/run/final/effect metadata, including ancestors. The
owning seal takes that complete union before checking parent access. These are
data/currentness fences and confer no execution or generic blocked-job authority.
The witness reference is also a metadata-only privacy edge. Delete/revoke and
latest-anchor restore follow ordinary dependencies and the reconciliation edge
recursively, physically erasing all dependent source/object/claim/cached copies
without turning a stale witness into a readable parent or deleting independent
primary ancestors.

For experience-bearing final publication the applicability supplement also calls the body-free Catalog
`CheckCapturedRunGrantsTx` primitive. The exact captured graph must include every
ordinary ancestor, including lessons used through derived presentations or
skills. Current run/session owner, task session/revision, exact source grants,
source ACL/revision/hash and fresh task/delegation/mandate authority remain
required. A missing ancestor pin or grant denies publication. This check returns
no bodies or new pins; it is not completion or inference authority by itself.
The owning completion checks its expected constraints revision before this call.

Completed independent owner verification and outbox operations retain their own
completed-result/owner-transition proof and applicability checks; they do not
borrow current inference access or compare the old run revision with a later
owner-completed task revision. Generic AccessTx and active provider/tool paths
retain their existing lifecycle restrictions. The older general Catalog
CheckRunSourcesTx checks source ACL/revision/privacy and delegation, not ordinary
nondelegated task grants. The reusable captured-grant regression documents that
separate boundary; this change wires the extra check only for experience-bearing
final publication, pending a separate review of general publication wiring.

At finalizing completion, captured-grant validation also requires the exact
current run token. After graph, grant and owner-authority waits it repeats a
plain metadata check of session/run/lease/fence identity and
`expires_at > clock_timestamp()`. The outer completion already holds its
ownership SHARE guard; this final check acquires no new lease lock, returns no
body and does not wait behind a queued takeover. A grant wait crossing lease
expiry denies the final while retaining the attempts. Completed independent
owner/outbox operations keep their own completed-result transition proof and do
not acquire a new live inference lease through this applicability supplement.


Migration082 permits only the exact content-free task working-state erasure
under an existing deletion of that session's owner. It preserves identity and
advances the working revision once. Stale experience observations still veto
ordinary publication; they cannot prevent a later deletion after an earlier
tool correction or privacy erasure. Any populated experience provenance refuses
downgrade: a pending external anchor need not have a committed SQL deletion yet.
