# Owned local indexing runtime

Runtime composes the source catalog, job-owned admission and derived semantic
store. `EnqueueWithPublication` saves the immutable finite owner job, new building
generation and index projection in one PG transaction. Job IDs are idempotent;
scope/limits/authority/source revisions must match. No orphan generation remains
after a failed admission/projection. MaxConns2 is supported without nested pool
borrowing. The worker claims only `index` jobs, retaining original run/task refs.

An owner job admits at most10000 chunks per generation. More current chunks than
its declared `max_chunks` fail before inference with `scope_chunk_limit`; no
partial generation activates and no automatic limit increase or retry is made.
The existing active pointer is unchanged. This is distinct from transport
unavailability. The synthetic capacity screen and runbook describe measured
Store limits separately from native indexing throughput and full user latency.

Each IPC reserves exact declared local pricing under current claim/source/task,
assigned goal/mandate and shared owner-day budgets. Its bounded context is used
for actual inference. The job context wraps every Rebuild read/publication.
Completed run leases never authorize post-run work; owner-created unassigned
finite jobs are a separate explicit policy. Task jobs still require active
current tasks. Missing authority, Secret material or unpinned/locality-unknown
targets deny. No cloud model is used by this runtime.

Durable vectors resume after process death; matching persisted text is not
reinferred. Transport unavailability retries at most 4 claims, 10 seconds apart,
under the original deadline/allowance. Source change/deletion or stale claims
deny late publication. A `done` job leaves the generation building: a separate
owner-proved review with measured quality artifact and expected previous pointer
verifies and activates it. Index completion never confirms memory claims.

`BUTLER_MEMORY_SEMANTIC_ENABLED=true` is an explicit operator switch. Composition
requires private service token file, reviewed `BUTLER_MEMORY_LOCAL_SERVICE_ID`,
the fixed `local-models:9090` endpoint, exact pinned embedding profile/revision and
`local:google/embeddinggemma-2` price `local_no_billing` in the existing execution
budget policy. A zero quote alone never proves locality. Use optional
`deploy/docker-compose.memory-semantic.yml` together with the local-models file;
only its single mode 600 token is mounted read-only, never the whole private tree.
Keep the current native/provider prices in the same policy; do not overwrite
them with a local-only example. The token never enters context/args/API/logs.

Live retrieval uses the same catalog and Controller with the actual current run
token; post-run indexing uses independent job claims. Ranking is off because the
measured Qwen corpus did not improve the embedding baseline. Legacy cloud
extraction/summary workers remain disabled and are unrelated to this runtime.

An authenticated owner may POST `/api/v4/memory/index-retire` with exactly one
`generation_id` and the scope's exact `expected_active_id` (empty only when no
pointer exists). This bounded maintenance path also works while local models and
the indexing runtime are disabled. It needs no backend or inference allowance.
The immutable index-job projection proves the owner; only a terminal
`done`/`failed`/`cancelled` job is eligible. Pending/running, foreign, ambiguous,
active or stale-CAS generations are rejected. Locks are job, generation, active
pointer. Current source privacy/revisions do not prevent derivative erasure.

One transaction removes only that generation's vectors and records irreversible
`retired_at`/`retired_by`. Generation, index-job, quality and spend history plus
original sources survive. Repeating the request returns the same retirement
timestamp and zero further removals. Retired generations cannot be rebuilt,
previewed, verified or selected for rollback. No automatic pruning, source
deletion, owner-scope widening or budget refund occurs. Retirement is derivative
maintenance in PostgreSQL; use ordinary Catalog deletion and its external anchor
for privacy erasure, including reconciliation of older restored backups.
