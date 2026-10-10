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
off. Current source/job/live-run admission is still required before enabling it;
the shared model service itself never grants that authority. See the semantic
module README and memory spec for these caller-owned contracts.

Service restarts lose no durable task state: callers retain pending operations in PostgreSQL, receive retryable RPC failure and
resubmit the full input; this service does not claim a durable work queue.
Full-local inference is tested with `--network none`; the production overlay has
an internal-only network and no published ports. A hybrid caller's cloud privacy
decision remains at its owned authorization/admission boundary. No service path
can route these inputs to a cloud provider, irrespective of caller/model output.
