# Local auxiliary models

The research-only taxonomy v2 catalogs pin explicit decision definitions and
priority rules with a 2048-token complete input cap. The worker verifies the
bundled `decision-schema-v2.json` hash before using it. Place that reviewed asset
beside `model-worker.py`; the Dockerfile and isolated runner do this explicitly.
Original profiles keep their original schemas. The v2 Qwen comparison uses a
separate pinned Ollama container through a test-only Go adapter, without a new
production route. GPU co-residency and explicit FP32 CPU EmbeddingGemma2 placement
are measured separately; see [the v2 experiment](../../deploy/local-models/decision-taxonomy-20261014/README.md).

Two text-only decision profiles are present but disabled in the default catalog.
The separate operator evaluation catalog enables pinned GLiNER Decide and d1
CPU FP32 solely for shadow comparison. Reviewed d1 code/config hashes are checked
before its local offline loader; complete prompt encoding must fit without
truncation. There are no decision-driven task/tool/verification hooks. See
`deploy/local-models/decision-code-review.md`; runtime support alone is not
Russian quality acceptance or a production model selection. The owned Go
shadow caller now has a fixed Russian comparison and complete actual CPU
artifacts; no model passes selection. An older replay also refused d1 repeat
loading under the then-current RAM accounting. Ordinary profiles remain disabled. See
[the caller contract](../../docs/architecture/local-decision-signals.md).

The separate full-score evaluation catalog adds CPU-only GLiNER profiles with
preprocessing `gliner2.5-classification-softmax-v1`. The pinned SDK's classification
scorer wraps the already loaded model and returns every requested label score.
Go rejects missing/duplicate/foreign alternatives and preserves the selected-only
profiles. This changes neither the ordinary catalog nor memory extraction. The
2026-10-14 isolated replay succeeds at repeat loading under the same limits,
including d1; historical resource refusals remain in the original artifacts.
Full scores still fail Russian quality/critical-error screening.

The optional versioned memory adapter is in `internal/memory/semantic`; it uses
this shared service with explicit query/document/code modes and 768/256d identity.
Production memory query/indexing stays disabled pending trusted run/job admission
and current-source access guards. A fixed Russian/code fixture and reproducible
quality/reindex checks are in `deploy/local-models/verify-quality-lab.sh`.

Go service for versioned embedding/ranking/extraction/classification/media
contracts and bounded local inference resources. It does not execute tools,
change memory facts, authorize actions or own durable tasks. Callers retain the
request in PostgreSQL and retry explicit Unavailable/ResourceExhausted responses.

Reviewed executable adapters are official EmbeddingGemma2 text/image vectors,
Qwen3-Reranker-0.6B and GLiNER2.5-multi-v1 through subordinate Python tensor workers.
gRPC auth, capability routing, round-robin caller queues, validation, cancellation,
GPU/RAM admission, idle unload and lifecycle remain Go. One serialized GPU lane
and a separate CPU GLiNER lane reuse workers across callers. Audio/video and
unsupported profiles return Unavailable; no empty successful fallback is invented.

The additive `butler-memory-v1` extraction schema returns source-span entities,
relations and action-anchored records with unresolved optional fields. It uses
the same pinned CPU GLiNER worker. Caller-owned extraction jobs and source checks
must publish only neutral candidates; the service cannot accept a profile fact or
authorize a mandate. See the architecture contract for Unicode span validation.

## Run on the lab

Use [the architecture contract](../../docs/architecture/local-model-service.md)
and [lab preparation](../../deploy/local-models/README.md). The optional Compose
profile is `deploy/docker-compose.local-models.lab.yml`; it publishes no ports and
uses an internal network. No orchestrator embedding setting is enabled by this
overlay. Dependency/model directories are prepared on the storage disk and
mounted read-only, rather than copied into Docker layers.

Required variables: `BUTLER_LOCAL_MODELS_TOKEN_FILE` (private 0600 file),
`BUTLER_LOCAL_MODELS_CATALOG` (operator-owned versioned JSON). Defaults: private
gRPC :9090; diagnostic HTTP :8080; 6144 MiB device budget; 512 MiB reserve; queue
16; idle unload 30 seconds; request deadline 60 seconds. Optional variables:
`BUTLER_LOCAL_MODELS_GRPC_ADDR`, `BUTLER_LOCAL_MODELS_HTTP_ADDR`,
`BUTLER_LOCAL_MODELS_BUDGET_MIB`, `BUTLER_LOCAL_MODELS_RESERVE_MIB`,
`BUTLER_LOCAL_MODELS_QUEUE_SIZE`, `BUTLER_LOCAL_MODELS_IDLE_SECONDS`,
`BUTLER_LOCAL_MODELS_GPU_INDEX`, `BUTLER_LOCAL_MODELS_PYTHON`,
`BUTLER_LOCAL_MODELS_WORKER`, `BUTLER_LOCAL_MODELS_MODEL_ROOT`.
RAM defaults to 6144 MiB with 512 MiB reserve; configure with
`BUTLER_LOCAL_MODELS_RAM_BUDGET_MIB`/`BUTLER_LOCAL_MODELS_RAM_RESERVE_MIB`.
Profiles require full-process peak RAM allowances; both lanes share an atomic
admission gate and sample total service RSS, host availability and cgroup headroom.
The latter credits only validated clean inactive file cache, subtracting dirty and
writeback bytes; missing or inconsistent counters deny admission. Hard container
and RSS limits and reservations remain in force; this performs no cache flush.

`/health` reports worker paths/backend availability; model availability is checked
per request against pinned metadata. `/metrics` has request/failure/queue/fallback,
OOM retry and whole-device peak counters/gauges, without input labels. All model
RPCs including catalog/status require bearer auth; diagnostics stay on the private
network. Neither request texts nor raw framework exceptions are logged.

## Validation

Run narrow Go tests/race on the lab, then `deploy/local-models/verify-lab.sh`.
The checked-in synthetic artifacts exercise real BF16 CUDA/FP32 CPU computation,
768/256 dimensions, auth, idle unload, cancel/recovery, concurrent index/search,
an 8000-byte batch, context rejection without truncation, and resource admission
fallback in a network namespace with no external interface. This is preliminary
runtime evidence; capability proof also checks genuine Russian ranking, spans,
classification, concurrent CPU/GPU lanes and on-demand image model loading.
Issue #25 still requires a production retrieval comparison,
versioned index migration, reversible reindex and larger held-out Russian/code
evaluation. It is not a claim that #24–#28 are all complete.

Optional typed PCM voice operations have three pinned default-disabled profiles.
See [voice preparation](../../deploy/local-models/VOICE.md) for public checkpoint
integrity checks, isolated SDK overlays, fixed audio/text bounds and the remaining
real quality/resource acceptance. ASR decoder completion carries no owner or
action authority. The service performs no microphone capture or input admission.

Optional `CAPABILITY_FUNCTION_PROPOSE` adds one fixed default-disabled CPU Q8_0
FunctionGemma catalog. It accepts bounded text/catalog version and returns only
candidate function/arguments, never executes tools. Operator-only
`BUTLER_LOCAL_MODELS_FUNCTION_EXECUTABLE` and `BUTLER_LOCAL_MODELS_FUNCTION_MODELS`
must point to prepared read-only pinned CPU/runtime and registry-model binds.
Absent configuration refuses this optional capability while ordinary tensor
health remains available. The Go-owned loopback child runs inside the existing
CPU fair queue, shared RAM gate and cgroup; cancellation kills its entire Linux
process group, idle unload removes it, and no inference pulls/cloud route exist.
See [function proposal experiment](../../docs/architecture/function-proposals.md)
for exact grammar, license, integrity and remaining quality/activation criteria.
