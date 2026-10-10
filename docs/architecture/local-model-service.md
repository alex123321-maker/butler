# Auxiliary local model service

This opt-in subsystem implements the resource-manager baseline for issue #24.
It is separate from Model Transport (the run's generative provider), Tool Broker,
Session Service, durable task ownership and the four memory stores. Auxiliary
model outputs are candidate data/signals, never approvals, success evidence or
instructions to execute external actions. PostgreSQL remains durable truth.

## Contract and ownership

`proto/localmodels/v1/localmodels.proto` defines authenticated internal gRPC:
catalog/status and Infer with typed embedding, rank, extraction, classification
and media inputs/outputs. The initial adapter supports text embeddings only;
unsupported profiles/capabilities return Unavailable. Source text stays transient
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

Go validates/clones input, enforces a finite queue and serial inference lane, and
applies one deadline to waiting plus execution. Canceled queued jobs are skipped.
Canceled active tensor work kills and waits for the owned Python process before
another worker can start. Service shutdown returns explicit retryable errors;
ephemeral queued input is recovered by the durable caller, never treated as a
completed task. Same-model/device workers are reused; changing models/device,
idle timeout or resource fallback unloads the owned process. Unknown resident
models/processes are never evicted.

## Resource policy

Initial GPU budget 6144 MiB, reserve 512 MiB, one selected NVIDIA device. Health
collects whole-device nvidia-smi total/used/free/name/driver, including other apps;
Ollama-reported resident bytes or tensor allocator bytes alone are insufficient.
Missing/invalid telemetry means GPU unavailable, rather than zero usage.
Admission checks budget minus reserve, free bytes, the profile's conservative
peak and current own residency. Execution samples whole-device memory every250ms
and after short calls. A resource overrun cancels the owned process; OOM can retry
the complete input with smaller batch, then FP32 CPU if the profile permits.
Input/context is never silently shortened and partial batches are not returned as
successful complete output. CPU failure remains explicit.

## Initial runtime and index identity

The supported initial weight is Google's public pinned EmbeddingGemma2 checkpoint,
text-only 270M component, Apache-2.0, BF16 GPU or FP32 CPU, native768 dimensions.
The model's activation range forbids FP16. MRL256 truncates then L2-normalizes;
all native components, including discarded tails, must be finite and nonzero.
Query/document/code-query prefixes are explicit typed modes with preprocessing
version `embeddinggemma2-retrieval-v1`. `index_space` includes profile/revision,
preprocessing version and dimensions. This identity must accompany every future
index entry/query; changing it requires side-by-side reindex and reversible
activation, not mixing vectors in existing memory columns. Existing Butler memory
settings/indexes remain unchanged by installing this service.

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
index/search, 8000-byte batches, explicit context rejection and constrained
admission in a namespace with no external interface. OOM is tested by injected
resource failures; unrelated GPU applications are not stressed or killed.

This baseline does not close the complete #24–#28 acceptance matrix: real
reranker/extractor/classifier/media adapters, CPU concurrent classification during
ranking, worst-case long-input/multimodal peaks, production quality comparisons,
durable caller/index integration and reversible reindex remain required. The
checked-in 12-case smoke is not a held-out production evaluation and does not
justify enabling the new embedding space for memory yet. Hybrid/full-local runs
remain caller concerns; this service has no capability to transmit input externally.
