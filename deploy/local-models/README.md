# Reproducible lab model runtime

All operations run on `ssh laba`, in a private source snapshot and
`/srv/storage/butler/agents/local-models`. Do not change the shared lab source
pointer or start the shared Compose stack from this directory.

1. Create the private root and sync with
   `scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/local-models`.
2. Run `bash current/deploy/local-models/prepare-lab.sh` on the lab. Network is
   allowed only during preparation. Public checkpoint download explicitly uses
   `token=False`, a full immutable HF revision, safetensors and data/config files;
   no remote Python code is downloaded or trusted.
   For a previously prepared embedding-only environment, use `extend-lab.sh`
   instead: the extra GLiNER libraries reuse installed Torch/CUDA wheels. The
   dry-run dependency plan and installation logs stay on the storage bind.
3. Run `bash current/deploy/local-models/verify-lab.sh`. It builds Go binaries on
   the lab and launches only `butler-local-models-proof`, using `--network none`,
   UID10001, read-only source/dependency/model mounts, no capabilities and private
   tmpfs. A random bearer token is generated in `private/token`, never printed.
   The fixture contains only public synthetic project statements and code.
4. Review `reports/verification-gpu.json`, `verification-constrained.json`, GPU
   CSVs and service logs. The helper unloads its own worker/container and records
   the device before/after. `constrained` or `gpu` as the second argument selects
   one mode; `capabilities` runs the complete reviewed model routing proof.
   It never unloads other applications' GPU processes.

The scripts use existing `butler-local-models-ollama` only for `ollama ps` evidence
from the isolated compatibility probe; it is not the inference backend. For a
fresh lab without that probe container, the helper records its absence. The
shared Butler Ollama is untouched. The optional Compose overlay is integrated
separately by the deployment owner after preparation; its service stays opt-in.

For the main lab stack, enable the overlay/profile using the private
`local-models/compose.env` file described in [deployment](../README.md).
`verify-compose-lab.sh` exercises authenticated RPC routing against that actual
service, verifies that anonymous catalog access is rejected, and checks its
internal-only network, absent published ports and runtime isolation. It reads
the token from a mounted file and contains only public synthetic inputs.

## Recorded hardware and results (2026-10-10 UTC)

Lab: Ubuntu kernel 7.0.0-34, RTX3060 Laptop 6144 MiB, driver595.91.07/CUDA13.2.
Torch2.10.0+cu128 is compatible with this driver's CUDA12 support. Main dependencies
and complete version lock are included here. The 271,002,624-parameter text model
loads BF16 on GPU and FP32 on CPU, omitting vision/audio encoders. Full downloaded
checkpoint occupies ~1.5 GiB; prepared framework/CUDA environment ~7.4 GiB.
All bytes are on `/srv/storage`, outside image layers. Installer temp/cache paths
also point to the storage bind; `pip --dry-run` can download large complete wheels.

Recorded GPU artifact: whole-device peak 1323 MiB for an 8000-byte four-document
batch (280 ms); small queries use about751 MiB. Cold index request 8.7 seconds,
warm small batches/queries 78–99 ms including final whole-device telemetry.
Cold CPU requests include ~8 seconds framework startup; the long CPU batch takes
8.9 seconds. Simultaneous indexing/search waits in the shared queue (80 ms GPU,
98 ms CPU). Over-context input receives InvalidArgument instead of truncation.
Twelve synthetic Russian/code queries retrieved the expected first
document (top1=1.0). These easy cases do not establish production quality or a win
against Butler's existing retrieval baseline. Constrained 1024 MiB manager budget
forces actual CPU fallback without allocating artificial OOM pressure. GPU usage
returns to the same 17 MiB baseline after workers stop. Periodic telemetry includes
other applications; finite/norm/dimension validation and explicit failure states
prevent a false successful empty embedding.

## Shared capability verification (2026-10-10 UTC)

The complete selected profile now includes the actual official
[Qwen3-Reranker-0.6B](https://huggingface.co/Qwen/Qwen3-Reranker-0.6B), revision
`e61197ed45024b0ed8a2d74b80b4d909f1255473`, and
[GLiNER2.5-multi-v1](https://huggingface.co/fastino/gliner2.5-multi-v1), revision
`cf5593a5d45e3bbf204b9df621b13b1c0cf25ee3`, alongside the existing Gemma2 checkpoint.
All weights occupy 3.7 GiB; the reused framework environment remains 7.4 GiB.
Additional libraries/dependencies are pinned in `extension.lock.txt` and the
complete installed `requirements.lock.txt`. No CUDA library is copied into an image.

`verification-capabilities.json` records real CPU Russian entity spans and label
scores, BF16 GPU ranking, overlapping CPU classification/GPU ranking (137/202 ms),
and loading Gemma2 text+vision 440M for a finite normalized 768d image vector.
Cold calls took 33.2 seconds GLiNER, 8.1 seconds Qwen and 10.2 seconds vision.
Peak whole-device use was 1935 MiB; full process RSS including both worker runtimes
peaked at 4394 MiB. The 6144 MiB RSS/device budgets each retain a 512 MiB reserve.
The service uses an 8 GiB hard container limit to include reclaimable file cache.
GPU returned to its unchanged 17 MiB baseline after container shutdown.

The image proof uses a synthetic public RGB image, not a claim of OCR/vision
quality; the Russian scores are runtime signals, not calibrated decisions or a
held-out evaluation. Audio/video remain unavailable. Production embedding,
reranking and extraction quality decisions belong to #25–#28.

## Versioned retrieval quality (2026-10-10 UTC)

Run `bash deploy/local-models/verify-quality-lab.sh /srv/storage/butler/agents/local-models`
after preparing the models/framework/private token above. It uses a separate
internal network, PostgreSQL fixture and bounded inference container. It checks
real pgvector storage, staged reindex/activation, and exact 768/256d query/document
spaces. Main memory, databases and active index pointers are untouched.

`retrieval-quality.json` fixes 24 public synthetic documents and 24 Russian/code
queries before tensor evaluation, including six code questions. The reference
answers distinguish changed agreements, similar but different errors, exact
approvals, ingress deduplication and permanent effect identities. This small
fixture is not a held-out benchmark or a claim about private owner documents.

| Retrieval mode | Expected source first | MRR | Recall at 4 |
|---|---:|---:|---:|
| Standard BM25 lexical control | 11/24 (45.8%) | — | — |
| EmbeddingGemma2 768d | 22/24 (91.7%) | 0.951 | 1.000 |
| EmbeddingGemma2 256d, normalized | 20/24 (83.3%) | 0.896 | 0.958 |
| 768d followed by Qwen reranking | 21/24 (87.5%) | — | — |
| 256d followed by Qwen reranking | 20/24 (83.3%) | — | — |

The real current title-keyword fallback found no first expected result in this
paraphrased fixture; the stronger standard BM25 control is reported separately.
The current cloud semantic backend is unavailable, so no comparison to its vector
quality is claimed. Embedding gain is reproducible; reranking regresses the 768d
result and stays optional/default-off. Similarity never establishes truth or
permission. Source refs, current revision/hash and typed data origin accompany
retrieved memory.

Latest `verification-retrieval-quality.json` records both-space indexing in
16.3 seconds including cold load, 48 query requests in 9.9 seconds and 48 rerank
requests in 15.1 seconds including a model switch. Earlier repeats took
9.9/4.0/12.1 seconds respectively; these totals include RPC/PostgreSQL validation
and host contention, rather than a promised latency. The final run admitted 108
calls and 113602 input bytes under a test-only 160-call/2MiB/eight-minute gate.
Whole-device peak was 2353 MiB and full process RSS 1989 MiB; device usage returned
to the same 17 MiB baseline. Concurrent GLiNER/GPU RSS evidence is recorded
separately in `verification-capabilities.json` above.

Actual PostgreSQL/race tests cover stale edits during publication, suppressed,
pending, expired and Secret sources, cross-scope exclusion, missing access
authority, restart-resumable batches, citation-only vector reuse, dimension
changes, activation CAS, still-current rollback and deletion cascades. Production
query/index admission and source access guards remain disabled until the owned
run/auxiliary-job integration from #22/#23 is verified; the fixture gate cannot
grant production authority. The optional adapter is documented in
`internal/memory/semantic/README.md`.

## Hybrid retrieval evidence (2026-10-10 UTC)

The same helper's second argument `hybrid` runs `HybridReader` through the actual
shared service and disposable pgvector fixture. It keeps the previously fixed
24 documents/queries and unchanged reference answers. `verification-hybrid-quality.json`
compares actual 768d embedding order with exact/simple-Russian full-text RRF and
the optional Qwen stage. Five exact function-name lookups succeed; a nonexistent
method returns no source and performs no semantic nearest-neighbor substitution.

| Mode | Top1 | MRR | Recall at4 |
|---|---:|---:|---:|
| 768d embedding | 22/24 | 0.951 | 1.000 |
| Protected exact + full-text + vector RRF | 22/24 | 0.951 | 1.000 |
| Hybrid + Qwen reranking | 21/24 | 0.931 | 1.000 |

The hybrid preserves paraphrased retrieval quality and adds exact technical
lookup/absence behavior; this corpus shows no natural-query top1 improvement.
Qwen is default-off because it regresses order. Expected-source recall measures
retrieval support; no generative answer accuracy or factual verification is
claimed. Task constraints and permissions still come from structured authority.

The recorded run used 8.7 seconds indexing, 2.1 seconds for 24 query calls and
9.8 seconds for 24 ranking calls with models grouped to profile quality. One full
serial query/rerank also ran through the public method and took 14.8 seconds,
including both GPU model switches; grouped timing is not the per-query production
latency. Whole-device peak 2353 MiB, full process RSS 1984 MiB, final GPU 17 MiB.
The largest cited context was 3819 serialized bytes against the 6144-byte budget.
Ranking transmits only bounded authorized text; clipped final evidence is marked
and does not silently shorten model input. Production stays disabled pending
the production source/job/live local-run composition.

## Owned semantic caller acceptance

`verification-owned-semantic.json` replays actual index/query/rank calls against
the already-deployed internal service. Every call has a committed shared budget
reservation; indexing uses an independent durable job claim, query/rank uses a
current live lease and confirmed local_only owner mandate. Catalog-derived chunk
links and revision/content stamps precede IPC and late output acceptance. The
smoke also verifies current citations, source deletion/vector cascade and denial
of publication into a still-building generation after deletion. This is a small
authority integration smoke, separate from the fixed24 quality evaluation.

Run `bash deploy/local-models/verify-owned-admission-lab.sh` on the lab with the
existing main service, isolated quality PostgreSQL, own source/cache root and
private single-file token mount. It launches no second tensor worker, attaches
only the disposable PostgreSQL to the internal network temporarily, disconnects
it afterward and writes a sanitized artifact. GPU idle unload remains the shared
service's normal policy. The main PostgreSQL/Compose source pointer is untouched.

The production #22 worker must wrap all Rebuild reads/publications in
`JobAdmission.Context`; bundle retrieval needs the live admission plus catalog
guard, exact local profile pricing and pinned internal connection. Missing hooks
remain denied. Post-run work retains original task/owner/source/authority pins;
completed-task indexing needs a new finite owner policy and remains unsupported.

## Main Compose verification
The main Compose service was exercised on 2026-10-10 at 22:01 UTC with actual
768/256-dimensional text embeddings, Russian extraction/classification, ranking
and image loading. See `verification-compose-capabilities.json`. GPU peaked at
1935 MiB and full service RSS at 4441 MiB; CPU classification overlapped GPU
ranking. Anonymous RPC access was rejected and the deployment helper verified
the sole internal network, no published ports, UID10001/read-only/capability
isolation and blocked public TCP egress. Vision initially received an explicit
resource refusal while the CPU extractor remained resident, then succeeded
after the ordinary 30-second idle unload with the same input and policy.
The artifact records this retry instead of reporting a false empty success.
## Structured extraction evaluation

`russian-extraction.json` fixes 24 public synthetic cases (6 development, 18
held-out), threshold 0.5 and known UTC event time. On the persistent internal
service, `verify-extraction-lab.sh` runs 24 structured CPU calls and 24 calls to
the existing generic entity adapter through actual finite owned-job admission.
Each complete source publication is retried from cache without another call.
All cases, including misses/errors, remain in the report; no cloud baseline call
is authorized by this script. Coordinate the shared inference window first.

The held-out structured model has precision 0.484/recall 0.564 versus generic
0.319/0.273 and rules 0.975/0.709. Five model prohibition omissions remain. The
predeclared quality gate fails and production scheduling stays off. Literal
constraint-fragment preservation is checked separately and excluded from these
metrics. A passed test runner/authority smoke is not quality approval. The
original empty-result and record-grouping failures are retained in earlier
artifacts; corrected storage and source-fragment handling do not tune the model.

The original run's cold call took 19.2s and warm structured calls 440–494ms versus
generic 136–158ms; whole process peak 2859MiB, GPU 0. Read the current artifact for
the repeated measurement, queue wait and every case, rather than treating grouped
warm calls as production latency. Exact exception/action polarity remains
unresolved; owner acceptance is separate. See the structured extraction contract.

## Tensor runtime exception

[Google's official card](https://huggingface.co/google/embeddinggemma-2) supports
Transformers/SentenceTransformers, text-only encoder selection and BF16/FP32;
FP16 is explicitly unsuitable. The card uses
[Apache-2.0](https://ai.google.dev/gemma/apache_2). We pin revision
`914f7f89142e33e77833254d9c9b90c3cef7303b` and reject version mismatches.

The official [Ollama BF16 text artifact](https://ollama.com/library/embeddinggemma-2:270m-bf16-text)
was also tested: Ollama0.40.2 tags identify revision
`afeb999480b2a5e48b590fc0583f8cbc7d206b74156feaead3a7840b81719295`,
format safetensors, runner `mlx`. Both GPU and `num_gpu:0` requests on Linux return
HTTP500 `MLX not available`. Its presence in the Ollama catalog does not mean it
runs on NVIDIA Linux. We retain that incompatibility evidence instead of silently
substituting the first EmbeddingGemma generation. Go delegates only tensor
computation to a managed subprocess; cancellation kills and waits for that process.

The official processor requires Pillow/torchvision even when vision/audio model
encoders are omitted. They are pinned; neither multimodal capability nor audio
quality is claimed. The runtime never resolves cloud models or follows network
fallbacks. Automatic generation-model loading is outside this service.
