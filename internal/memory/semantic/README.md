# Versioned local semantic retrieval

`Adapter` calls the shared authenticated local-model service, never loads weights
or calls a cloud endpoint. The composition root must provide the internal local
connection and a trusted `Admission`; its absence blocks every inference. Each
operation carries source origin, scope, fresh call ID, bytes, items and timeout.
Admission returns the bounded context used by the actual IPC as well as a release
function. A missing/expired or weakened deadline denies the call; a late result
after admitted cancellation is rejected.
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

## Hybrid retrieval

`HybridReader` combines current PostgreSQL exact identifiers/paths/versions and
Russian/simple full-text candidates with versioned vector candidates. Migration042
adds the derived GIN expression index. Scope/live-state/Secret checks and the
source guard precede transmission of candidates. A bare technical identifier
uses exact matching only; an absent method does not produce a nearest unrelated
semantic answer. Identifier case and Unicode paths are preserved.

Lists are merged by reciprocal rank positions (`k=60`), with exact hits protected,
not by adding PostgreSQL ranks, cosine distances or model scores. Equal content
is deduplicated without folding code case. A source contributes at most two of
four final candidates. Candidate lists are each bounded at 16 per scope (at most
three scopes); reranking uses at most four candidates and 8192 input bytes under
the shared service's measured 2048-token context. Larger packs keep base results
and report `reranker_input_limit`; model input is never silently truncated.

Reranking is per authorized scope; cross-scope RRF positions remain. Unavailable
embedding/ranking produces fixed observable degradation reasons and permitted
lexical/base results. Admission/access/Secret denial, cancellation, malformed
dimensions or partial/corrupt model scores remain errors and never fall back.
Sources are checked again before ranking and before publishing its result.
Only text representations enter the text reranker; raw media has no request field.

Final evidence carries chunk/document refs and a current revision. Explicit
UTF-8-safe excerpts are at most 1024 bytes each; retrieved JSON is bounded at 6144
bytes by default (operator can lower it, hard maximum 8192). Its UTF-8 byte count
is a conservative token allowance for byte-token encodings, not actual tokenizer
usage or the entire model prompt budget. Caller prompt serialization still needs
the independent provider/budget guard. Clipped excerpts are marked in the bundle.

`SearchDetailed` returns diagnostics and resource observations. The bundle-facing
`Search` also logs fixed degradation enums without queries/source text. The fixed
real Russian/code evaluation preserves 22/24 top1 and 24/24 recall at 4; Qwen lowers
top1 to 21/24 and remains off. Expected-source coverage measures retrieval support,
not generative answer correctness. Current source/job/live local-run integration
is still a prerequisite for production activation.

Sources: [PostgreSQL full-text controls](https://www.postgresql.org/docs/17/textsearch-controls.html),
[RRF paper](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf),
[official Qwen card](https://huggingface.co/Qwen/Qwen3-Reranker-0.6B).
