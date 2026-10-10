# Owned local indexing runtime

Runtime composes the source catalog, job-owned admission and derived semantic
store. `EnqueueWithPublication` saves the immutable finite owner job, new building
generation and index projection in one PG transaction. Job IDs are idempotent;
scope/limits/authority/source revisions must match. No orphan generation remains
after a failed admission/projection. MaxConns2 is supported without nested pool
borrowing. The worker claims only `index` jobs, retaining original run/task refs.

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
