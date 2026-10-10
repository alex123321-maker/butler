# Versioned local semantic retrieval

`Adapter` calls the shared authenticated local-model service, never loads weights
or calls a cloud endpoint. The composition root must provide the internal local
connection and a trusted `Admission`; its absence blocks every inference. Each
operation carries source origin, scope, fresh call ID, bytes, items and timeout.
Secret origins and known credential patterns are denied before model IPC.
Legacy origins remain private data; provenance never grants instruction authority.

`Store` keeps retrieval derivatives in migration041. Memory chunks remain durable
truth. Immutable `Space` includes model/revision/runtime/preprocessing/dimensions
and chunk version. The official adapter selects query/document/code-query modes,
accepts BF16 GPU or FP32 CPU, and checks every vector's correlated ID, dimension,
finite components and L2 norm. The existing memory vector columns are untouched.

Every source read, ranking candidate and publication also requires `SourceGuard`;
its absence denies access. The #22 adapter must derive current catalog links and
owner/run/job authority from PostgreSQL chunk IDs. Stored `catalog` provenance
fields only describe citations. The transaction hook locks sorted catalog and
authority rows before this store locks chunks, using the supplied transaction
without another pool connection.

`Rebuild` resumes a caller-owned generation after interruption and reuses matching
persisted entries. Sources are bounded; oversized data requires #23 chunking and
cannot be silently truncated. Source revision/hash and live scope/confirmation,
status, suppression and expiry are checked before persistence and retrieval;
after reranking a live source is rechecked. Source deletion cascades to vectors.
An unchanged title/content hash can reuse a vector from the exact same space into
a new generation after a citation/path change. This refreshes revision-bound
citations without another model call; changed content never matches that hash.

Staged `Preview` evaluates without activation. `Verify` requires complete current
coverage and a trusted reviewed quality artifact showing an improvement. `Activate`
uses the expected previous ID; the same primitive rolls back to a still-current
ready generation. An edited source makes old generations unsuitable for rollback.
Ready generations cannot be mutated, and downgrade refuses retained history.
The artifact reference/hash is a review record, not an automatic external proof
reader or a declaration that similarity establishes factual truth.

`Reader` optionally plugs into the memory bundle with source refs/revisions and
typed private origin. No bootstrap enables it yet: current run-owned local query
admission and durable auxiliary job-owned indexing admission are #22 integration
dependencies. A completed run lease must never be resurrected for a memory job.
The source guard implementation and full acceptance/contradiction/tombstone/restore semantics and
structured ingestion remain #22/#23 responsibilities. This module currently
indexes authorized memory chunks; it does not independently scan the computer.

Real fixture evaluation and replay are in `deploy/local-models/`; production
quality across private documents or comparison to unavailable cloud embeddings
is not claimed. Reranking is optional and default-off.
