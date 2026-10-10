# Local auxiliary models

Go service for versioned embedding/ranking/extraction/classification/media
contracts and bounded local inference resources. It does not execute tools,
change memory facts, authorize actions or own durable tasks. Callers retain the
request in PostgreSQL and retry explicit Unavailable/ResourceExhausted responses.

The initial executable adapter is official EmbeddingGemma2 text inference through
a subordinate Python tensor worker. gRPC auth, queueing, validation, cancellation,
GPU admission, idle unload and lifecycle remain Go. Ranking, extraction,
classification and media inputs are typed but return Unavailable until a reviewed
adapter/profile is installed; there are no fabricated empty successful results.

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
runtime evidence; issue #25 still requires a production retrieval comparison,
versioned index migration, reversible reindex and larger held-out Russian/code
evaluation. It is not a claim that #24–#28 are all complete.
