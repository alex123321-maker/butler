# Auxiliary local model service

This opt-in subsystem implements the shared auxiliary resource service for #24.
It is separate from Model Transport (the run's generative provider), Tool Broker,
Session Service, durable task ownership and the four memory stores. Auxiliary
model outputs are candidate data/signals, never approvals, success evidence or
instructions to execute external actions. PostgreSQL remains durable truth.

## Contract and ownership

`proto/localmodels/v1/localmodels.proto` defines authenticated internal gRPC:
catalog/status and Infer with typed embedding, rank, extraction, classification
and media inputs/outputs. Reviewed adapters support text embeddings, reranking,
entity extraction, classification and bounded PNG/JPEG image embeddings.
Unsupported profiles/modalities return Unavailable. Source text stays transient
in a bounded request/queue; it is neither logged nor persisted by this service.
The caller owns durable work, scope/permission filtering, retry and storing model
revision/provenance. There is no cloud fallback or tool execution endpoint.

Only operator-installed profiles choose weights, runtime, immutable revision,
format, precision, source/license, preprocessing version, dimensions, input/batch
limits, context and an explicit full-device peak admission allowance. Inputs cannot
select executable paths, remote code, a model URL, a working directory or network
credentials. Configured helper paths remain outside layered/model-editable config.
Catalog/status contain metadata, not credential material. All model RPCs require
the private token-file bearer credential; raw errors never cross the boundary.

Go validates/clones input, enforces finite round-robin caller queues and two lanes:
one serialized GPU/model-switch lane and one CPU-only extraction/classification
lane. CPU work can overlap GPU ranking; no two GPU inferences or loads overlap.
Each lane owns one reusable worker, independent of the number of calling agents.
FIFO is preserved per caller; catalog-selected CPU models cannot enter the GPU
lane. Omitted profile IDs route only if one enabled profile provides the requested
capability; ambiguity fails explicitly. A single deadline covers waiting and work.
Canceled queued jobs are skipped.
Canceled active tensor work kills and waits for the owned Python process before
another worker can start. Service shutdown returns explicit retryable errors;
ephemeral queued input is recovered by the durable caller, never treated as a
completed task. Same-model/device workers are reused; changing models/device,
idle timeout or resource fallback unloads the owned process. Unknown resident
models/processes are never evicted.

## Resource policy

GPU budget 6144 MiB, reserve 512 MiB, one selected NVIDIA device. Health
collects whole-device nvidia-smi total/used/free/name/driver, including other apps;
Ollama-reported resident bytes or tensor allocator bytes alone are insufficient.
Missing/invalid telemetry means GPU unavailable, rather than zero usage.
Admission checks budget minus reserve, free bytes, the profile's conservative
peak and current own residency. Execution samples whole-device memory every250ms
and after short calls. A resource overrun cancels the owned process; OOM can retry
the complete input with smaller batch, then FP32 CPU if the profile permits.
RAM budget independently defaults to 6144 MiB with a 512 MiB reserve. A shared
admission gate prevents simultaneous cold loads from promising the same RAM.
CPU fallback uses a separate conservative FP32 RAM allowance from each profile;
insufficient RAM rejects admission before tensor allocation.
Telemetry sums full process RSS in the isolated container PID namespace, including
both workers/frameworks, rather than reporting tensor allocations alone. Shared
pages can be counted twice conservatively. Host MemAvailable and finite cgroup
headroom are checked too. Unknown telemetry denies admission; a sampled overrun
cancels and unloads the owned worker. The container also has an 8 GiB hard memory
limit without extra swap; cache pages may be reclaimed independently of RSS.
Input/context is never silently shortened and partial batches are not returned as
successful complete output. CPU failure remains explicit.

## Initial runtime and index identity

The embedding weight is Google's public pinned EmbeddingGemma2 checkpoint,
text-only 270M component, Apache-2.0, BF16 GPU or FP32 CPU, native768 dimensions.
The model's activation range forbids FP16. MRL256 truncates then L2-normalizes;
all native components, including discarded tails, must be finite and nonzero.
Query/document/code-query prefixes are explicit typed modes with preprocessing
version `embeddinggemma2-retrieval-v1`. `index_space` includes profile/revision,
preprocessing version and dimensions. This identity must accompany every future
index entry/query; changing it requires side-by-side reindex and reversible
activation, not mixing vectors in existing memory columns. Existing Butler memory
settings/indexes remain unchanged by installing this service. Its separately
selected image profile loads the text+vision 440M components on demand and omits
audio; only PNG/JPEG up to 512x512/1 MiB are accepted. This is an image vector
capability, not OCR, image description, audio or video recognition.

The pinned Qwen3-Reranker-0.6B adapter uses its official local chat template and
yes/no logits, BF16 GPU or FP32 CPU, and returns a complete correlated candidate
score set. The pinned multilingual GLiNER2.5-multi-v1 boundary extractor runs FP32
on CPU, returns validated half-open Unicode-character entity spans or selected
label scores. All three models are Apache-2.0; IDs/revisions, dependency versions,
preprocessing versions and conservative measured admission allowances are in the
operator-owned catalog. Scores are uncalibrated model signals, never authority.

Python is an explicit tensor-runtime exception: the official safetensors model
architecture is available in PyTorch/Transformers/SentenceTransformers, whereas
the tested official Ollama artifact uses an unavailable MLX runner on NVIDIA
Linux. No Python orchestration, task state, auth decisions or tool broker logic
is introduced. Dependencies are version-locked on a storage bind, model files
are pinned/downloaded before inference, and the worker enforces local-only,
`trust_remote_code=False`, safetensors, read-only model directories. Inference
has no internet route. Model/worker startup can take seconds; warm workers are
shared rather than copied per agent.

## Evidence and remaining scope

`deploy/local-models/` contains setup, replayable isolated verification, public
Russian/code fixtures and measured GPU/CPU artifacts. Unit/race contracts cover
serial queues, cancellation, over-budget/OOM fallback, idle unload, unknown
residents, normalization and authentication/error redaction. Real smoke covers
BF16 CUDA/FP32 CPU, dimensions, cold cancel/recovery, idle unload, concurrent
index/search, 8000-byte batches, nearly 2048-token documents, explicit context
rejection and constrained
admission in a namespace with no external interface. OOM is tested by injected
resource failures; unrelated GPU applications are not stressed or killed.

Real capability evidence includes Russian spans/classification and ranking, CPU
classification during GPU ranking, and on-demand vision loading. The checked-in
12-case embedding smoke and small capability signals are not a held-out production
evaluation and do not justify enabling a new memory index or closing #25–#28.
Quality comparisons, durable caller/index integration and reversible reindex
remain separate acceptance work.
The optional semantic/hybrid caller now has reproducible fixed Russian/code
quality artifacts, versioned staged pgvector spaces and bounded exact/full-text
RRF retrieval. The paired Qwen stage regresses top1 on this fixture and remains
off. Current source/job/live-run admission is required before enabling it;
the shared model service itself never grants that authority. See the semantic
module README and memory spec for these caller-owned contracts.

The semantic caller now supplies actual catalog-derived source guards, independent
owned indexing job admission and a dedicated local live-run budget boundary.
Source stamps pin chunk ID/scope/revision/full-content and embedding hashes across
batch waits; current state is checked before IPC and before returned output is
accepted. Local-only mandate privacy is allowed only on the composed internal
service, independently of an explicit local_no_billing quote. The real
`verification-owned-semantic.json` artifact covers three actual calls on the same
deployed service with shared reservations, citations and deletion fences. This
does not automatically enable production ingestion or grant completed-task jobs.

Service restarts lose no durable task state: callers retain pending operations in PostgreSQL, receive retryable RPC failure and
resubmit the full input; this service does not claim a durable work queue.
Full-local inference is tested with `--network none`; the production overlay has
an internal-only network and no published ports. A hybrid caller's cloud privacy
decision remains at its owned authorization/admission boundary. No service path
can route these inputs to a cloud provider, irrespective of caller/model output.

## Structured extraction contract

Two optional text-only classification profiles support a default-off decision
experiment. GLiNER Decide uses the existing pinned SDK; d1 uses one explicitly
reviewed immutable upstream revision with exact code/config hashes checked
through bounded contained reads before the offline local loader. Its complete
state/options encoding is compared with upstream encoding so silent truncation
is refused. CPU-only FP32 is configured, with no image/audio interface or cloud
fallback. Only an operator evaluation catalog enables these profiles. Signals
cannot change task control, action authority or result verdict; Russian quality
and model selection remain separate measured caller work.

`ExtractInput.schema_version=butler-memory-v1` selects a fixed reviewed data
schema and requires an empty `entity_types` list. It returns additive typed
entity spans, action/resource/deadline/exception and project/component relations,
and action-anchored constraint records. Every field is a literal half-open
Unicode-character span with an uncalibrated score; malformed, generated or
missing-anchor records are rejected. Optional/ambiguous fields remain absent or
explicitly unresolved. This is independent decoding, not a guarantee that every
relationship is correct, and all output stays candidate data. The worker uses the
already pinned GLiNER2.5 revision/runtime without new dependencies or remote code.
Existing generic entity extraction remains compatible. Caller source/job/budget
checks and owner acceptance are separate; this contract alone enables no memory
pipeline. Real Russian quality and production integration require caller evidence.

The optional Go extraction caller now has current source/job admission, atomic
candidate publication and literal fragment evidence. Its fixed Russian model
quality gate fails, so automatic scheduling remains disabled. A separate
source-aware constraint-fragment candidate preserves literal restrictions when
model record grouping fails, with action polarity and exception scope explicitly
unresolved. Model field metrics exclude that protection layer. See
[structured candidate extraction](structured-memory-extraction.md).

An operator-only evaluation catalog additionally selects
`gliner2.5-classification-softmax-v1` for the two pinned CPU FP32 GLiNER checkpoints.
The upstream `Classifier` facade wraps the already admitted resident model;
`ClassificationSchema.single` fixes softmax activation and `score` returns all
requested alternatives. Complete text plus compiled schema is checked against
the 512-token cap before scoring with `max_len=None`. No second checkpoint,
dependency, remote code or model-selected decoding option is introduced. Go
validates the complete correlated label set, finite bounded scores, revision,
runtime and preprocessing identity. Selected-only profiles keep their original
behavior. These scores remain uncalibrated signals. The default catalog does not
include the new profiles; no automatic decision consumer is enabled.

The default-off decision caller records real source/job-admitted CPU signals
from all three pinned models on a frozen Russian corpus. None passes selection.
Historical repeats retain d1 RAM admission refusals; the 2026-10-14 isolated
replay succeeds at all repeated loads under the current accounting and unchanged
limits, while both selected-only and full-score profiles fail quality screening. Signals
cannot suppress events or acquire task/grant/verification authority. See
[local decision observations](local-decision-signals.md) and the checked-in
acceptance matrix for remaining current-controller baseline work.

Default-disabled voice profiles add fixed `ASR`, `VAD` and `TTS` typed operations.
Inputs use canonical mono PCM16 WAV at 16 kHz; ASR is bounded to 15 seconds per
actual request and VAD to 120 seconds. TTS accepts at most 200 Unicode characters
and returns canonical 24 kHz PCM16 with a 20-second output bound. No compressed
codec, arbitrary sample rate, reference voice, cloning, model path, speaker or
runtime setting is supplied by a request. Qwen ASR/TTS revisions and the Silero
Git revision are pinned; all libraries, weights and preparation caches remain on
the storage bind. Offline SDK library overlays resolve upstream incompatible
Transformers/NumPy versions without changing the existing embedding runtime.
The optional Python worker is justified by official Qwen/Silero inference SDKs;
Go owns admission, routing, deadlines and typed response validation.

`decoder_complete` only records a decoder EOS. It is neither a correctness claim
nor owner authentication. The local service never accepts a transcript as an
ordinary input or action authorization. Voice source persistence, explicit owner
review and the normal durable input/Stop contracts belong to the separate caller.
All three profiles remain disabled until their caller and real quality/resource
acceptance are complete. The evaluation catalog enables only those three optional
profiles; it must be deliberately staged and then removed by the operator.

On cgroup v2, RAM admission bounds host `MemAvailable` by the hard
`memory.max` headroom plus clean `inactive_file` cache. Required `memory.stat`
keys are validated by name; missing, malformed, duplicate or impossible counts
deny admission. Credit is bounded by both `file` and `memory.current`, and all
`file_dirty` plus `file_writeback` bytes are conservatively deducted from inactive
cache. Anonymous and active file memory remain fully charged. This estimates
reclaimable cache, not guaranteed future availability: the 8 GiB container cap,
6 GiB process-RSS policy, 512 MiB reserve and shared atomic lane reservations
still apply. No cache flushing is performed. The accounting follows the
[kernel cgroup v2 counters](https://docs.kernel.org/admin-guide/cgroup-v2.html)
and the [Docker inactive-file accounting convention](https://docs.docker.com/reference/cli/docker/container/stats/),
with an additional conservative dirty/writeback subtraction. Earlier voice
attempts rejected under raw cgroup charges remain recorded as resource refusals.

The optional fixed `FUNCTION_PROPOSE` operation uses a pinned public FunctionGemma
Q8_0 registry release and a Go-owned Ollama CPU subprocess inside the existing
service PID namespace/cgroup. It shares the ordinary serialized CPU lane, fair
queue, RAM reservations, deadline cancellation and idle unload. It does not use
the separately deployed Ollama service. A fixed Google control-token formatter
and complete-set output validator normalize candidates only. Model/runtime paths
are operator composition, never request fields; runtime/model blobs and library
symlinks are checked against pins before the read-only offline load. The normal
profile remains disabled and no orchestrator command route is enabled. Current
source/job admission, Broker schema/permission/task revision and measured Russian
quality belong to its caller; see [function proposals](function-proposals.md).
