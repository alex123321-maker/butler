# Memory source catalog

Delegated runs select only their exact contract/source roots and their own
session/task bodies. Original dependency ancestors remain privacy pins rather
than additional selectable context. Catalog owns same-transaction child grants
and rechecks the current parent grant chain at reads, dispatch and publication;
copied child grants cannot retain revoked parent permissions.

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

`ProposeTx` supports trusted structured extraction checkpoints and exact retry
keys in the caller transaction. It rechecks current owner/job/source access,
publishes only candidates and rejects a changed body under an existing key.
`DedupKey` is not an HTTP/model field; ordinary owner manual proposals keep their
random identity. Migration043 preserves only opaque IDs/hashes/times separately
from source/claim text, including after catalog deletion.

`VerifiedResultParents` is a content-free historical evidence bridge for reviewed
skills. It binds the actual assistant message to its run, checks restore identity
and every original run/source edge, and returns the exact final-message root.
It grants no publication authority. `CreateDerivedTx` requires a fresh current
owner/run/job proof for all parents; deletion of any original dependency erases
the procedure body while content-free skill audit remains in migration051.
