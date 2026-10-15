# Versioned local semantic retrieval

Policy-aware bundle reads distinguish local query use from external release.
For local-only requests `HybridReader.SearchWithPolicy` permits PostgreSQL exact
and full-text reads, while inference requires the backend's private pinned target
to match the current `LiveAdmission` target/identity. Unproven targets never
receive query text or reranker snippets; lexical evidence may still be returned
with an explicit degraded diagnostic. Genuine source/run failures propagate.
Neither local lookup nor zero incremental billing grants cloud generation.

`Adapter` calls the shared authenticated local-model service, never loads weights
or calls a cloud endpoint. The composition root must provide the internal local
connection and a trusted `Admission`; its absence blocks every inference. Each
operation carries source origin, scope, fresh call ID, bytes, items and timeout.
Index/rank operations additionally bind each chunk's ID, scope, revision, full
content/citation hash and title/text embedding hash. Rebuild supplies the typed
snapshots from its pending batch; the exact serialized payload must match.
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
its absence denies access. `CatalogGuard` derives current catalog links and
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
typed private origin. `LiveAdmission` now reserves current run-owned local query
and ranking attempts; `JobAdmission` reserves independent durable job-owned
index attempts. Both use current PostgreSQL authority and check source snapshots
before IPC, after allowance waits, during in-flight calls and before accepting
returned vectors/scores. A completed run lease is never resurrected for a job;
completed-task jobs still require a separate finite owner policy and are denied.
No bootstrap enables this module automatically. The production job worker/bundle
composition and full acceptance/contradiction/tombstone/restore semantics and
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
not generative answer correctness. The checked-in owned-admission smoke proves
real index/query/rank calls through current catalog, local-only owner mandate,
independent job/live lease and shared durable allowances on the same deployed
service. Production activation still requires explicit #22 worker/bundle wiring.

## Trusted composition

The composition root creates `budget.NewLocalTarget("local-models:9090", identity)`
from fixed deployment configuration, and `NewPinnedBackend(target, tokenFile)`
using a single private mode0600 regular token file. The token stays in RPC metadata;
symlinks, public permissions and oversized files are rejected. The Docker internal
network plus pinned deployed service/profile is the locality trust boundary; this
is not a claim of public TLS endpoint attestation.

Create `NewCatalogGuard(catalogStore, purpose)` and assign it to `Store.Guard`.
The budget controller installs the task owner's `ResolveBudgetAuthority` and
declares exact `local:google/embeddinggemma-2` / `local:Qwen/Qwen3-Reranker-0.6B`
prices with explicit `local_no_billing`. That monetary declaration grants neither
locality nor task/source authority. `NewLiveAdmission` is query/rank only. For an
index worker, `NewJobAdmission` binds the actual claimed job and its fixed purpose;
use its `Context` for **all** Rebuild reads/publications, and pass it as the
adapter Admission. Unknown/missing pricing, target, claim, lease, current source
or assignment fails closed. Rerank remains default-off.

All authority acceptance tests apply every migration in isolated schemas with
MaxConns2. Earlier index/quality tests use separate pre-catalog schemas and an
explicit fixture-only guard; they do not count as production authorization proof.
`deploy/local-models/verify-owned-admission-lab.sh` replays the real same-service
smoke without launching another inference worker or touching main PostgreSQL.

Sources: [PostgreSQL full-text controls](https://www.postgresql.org/docs/17/textsearch-controls.html),
[RRF paper](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf),
[official Qwen card](https://huggingface.co/Qwen/Qwen3-Reranker-0.6B).
