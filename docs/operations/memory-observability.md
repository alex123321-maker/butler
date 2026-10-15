# Memory observations and Doctor

Generative background jobs add owner-proved `/api/v4/memory/model-jobs` observations
and policy/job/worker controls. Doctor includes persisted model control, attempt
count, unknown usage and reserved tokens/cost; corresponding `butler_memory_durable_model_*`
gauges have no new personal identifiers or text labels. They report accounting,
not inference quality or worker liveness. See [background jobs](../architecture/background-memory-jobs.md)
for locality waiting, active-hours pauses, queue limits, cancellation and rollback.

Doctor distinguishes configuration, connected implementation, a timestamped
observation, request outcome and quality evidence. A configured object is not a
successful backend probe. PostgreSQL owns durable memory, auxiliary jobs and
index generations; the legacy Redis extraction queue is displayed separately.

## Operator check

Open `/doctor`, enter the provisioned owner-control token and select **Run System
Check**. The token remains in page memory and is cleared when it changes or the
page closes. Loading historical reports also requires owner authentication.
`POST /api/v1/doctor/check` with `{}` atomically admits a real diagnostic task
and accepted input in a new Session Service session. It returns 202 and
`report.input_id`. Poll the same endpoint with `{"input_id":"<receipt>"}` using
the same owner proof; refreshing never dispatches another check. The pending
receipt is stored in report history, so leaving for Approvals does not lose it.
Polling advances that pending snapshot; the final snapshot is stored once.

The existing input worker acquires its real run/session lease. A private input
checkpoint marker selects the fixed `doctor.check_system` command with `{}`;
request payload fields cannot select this path. Broker preparation, exact owner
approval where required, authorization and one-use dispatch remain mandatory.
In mode 1, open **Approvals**, confirm the exact action, then refresh the check.
Worker restart preserves the decision and original action. No model request,
provider cancellation, benchmark or new background worker is needed. Unknown
dispatch outcomes remain blocked and are never automatically replayed.

Completion returns a stored report even when runtime checks or memory reporting
fail. `check_error` distinguishes transport/unavailable execution, authorization,
ToolResult failure, invalid/missing results and stale observations. Independent
memory checks survive other failures. A successful HTTP response means that an
observation was delivered; inspect `status`, `check_error` and individual checks.
Historical reports retain their JSON shape and original observed times.

## Reading memory state

`memory.components` lists `configured`, `effective`, `required`, `status`,
`reason` and `checked_at`. Store health means its fixed SQL read probe completed,
including on an empty table; it does not prove source access or answer quality.
Current task Working state and the legacy Working table have separate probes.
The deletion manifest is checked separately. Semantic reader composition and
legacy embedding/pipeline composition are separate capabilities. A disabled
legacy pipeline cannot hide a connected semantic reader. Inference and worker
liveness remain `unknown` until observed; merely enabling them cannot prove
health. Missing/expired observations cannot aggregate to full `healthy`.

`jobs.observation` contains owner-scoped pending/running/done/failed/cancelled
counts, retry attempts, oldest pending age and bounded stored reason groups.
`indexes.generations` contains at most 50 scope-checked generations, their build
state, verification timestamp, active pointer and stored generation proof.
Index build completion, quality verification and activation are different
states. A query failure omits the measurement instead of reporting zero. Probe
deadlines are one or two seconds, with an eight-second memory report bound.
Scope checking never grants access and telemetry performs no job mutation.

Generation proof includes its artifact reference/hash, corpus/method comparison
and verification date. It applies to that generation, not to assistant answer
quality. The current runtime has no accepted, configuration-bound assistant
quality reference store: `answer_quality.status` is **unverified**, with the
actual configuration fingerprint. Stored #78 evaluations are not imported as
production verdicts. A proof from a different configuration cannot establish
current answer quality; inspect its original configuration/scope/limitations.

## Metrics and interpretation

The ordinary context-building path uses the composition root's registry at
`/metrics`. New metrics have fixed stages/dispositions and enumerated outcomes
or reasons; they never label queries, text, owners, source IDs, job IDs or index
generations. Error messages and provider/SQL bodies are not copied into labels.

- `butler_memory_operations_total{stage,outcome}` and
  `butler_memory_operation_duration_seconds{stage}` cover bundle, search,
  document search, context selection and profile/working/summary/episodic reads.
  The same bounded observations accompany each request's SelectionDiagnostics,
  including failures of optional reads that preserve partial context.
  A started stage finishes once,
  including denial, cancellation and failure. A stage not reached has no event.
- `butler_memory_items{disposition}` observes optional candidates found,
  selected and excluded. Histogram sums retain item counts; this is not a
  tokenizer or a measurement of the complete final provider request.
- `butler_memory_limitations_total{stage,reason}` counts a reason once per
  operation, including missing semantic index, SQL fallback, context allowance
  and execution budget. Unknown backend diagnostics map to `unknown`.
- Existing `butler_memory_retrievals_total{memory_type,service,status}` retains
  its selected-class `hit`/`miss` meaning. New stages do not redefine it.
- `butler_memory_durable_*` gauges read global content-free PostgreSQL
  aggregates on scrape, with a two-second deadline: pending, running, completed,
  failed, retries, oldest pending age and building/verified/active generations.
  These recover after process restart without a worker first running.
  `observation_success=0` omits backlog/index values. `observed_at_seconds`
  exists only for successful observations. Legacy Redis queue values remain
  separately named and missing values are not zero.

For a request, `empty` requires positive knowledge of an empty permitted corpus;
`no_match` requires known permitted data with no matches. A zero result alone is
`unknown`. Current document search does not enumerate inaccessible sources to
distinguish these cases; its optional corpus observation can remain unknown.
`disabled` is intentional query suppression; `denied` is an access decision.
`stale`/`revoked` name source restrictions; `context_budget` names context
omissions and `execution_budget` names rejected inference admission. SQL results
remain successful while a semantic limitation is recorded separately.

## Initial operator signals

These are diagnostic starting points, not measured SLOs (#91):

- Alert on `observation_success == 0` or absent observations for five minutes.
  Treat `time() - observed_at_seconds > 300` as stale, never as a zero queue.
- Investigate a pending backlog whose oldest age exceeds five minutes and whose
  completed count does not advance over ten minutes. Check worker effective
  state, deadlines, attempts and stored reasons before considering retries.
  A disabled legacy worker with a nonzero legacy queue needs operator review.
- Investigate any increase in failed jobs or `outcome="failed"` over five
  minutes. Separate access denials/stale sources from backend failures; never
  retry a revoked source or override the deletion manifest to clear an alert.
- Inspect search/context latency distributions when p95 exceeds one second
  over ten minutes, and compare context exclusions and semantic fallback rate.
  These provisional signals do not authorize raising budgets or enabling a
  pipeline. Use #91 to establish measured limits and #92 for system readiness.

## Reproducible isolated acceptance

From Windows, use `scripts/dev/verify-changed.ps1` and targeted
`scripts/dev/test-go.ps1 -Packages ... -Run ...` first. Sync only into a dedicated
agent root, then run `memory-observability-verify-lab.sh` there. It uses a private
Docker network, separate pgvector database, actual Broker process and controlled
runtime; the owner decision/restart and missing-grant rejection are asserted.
It does not change the deployed stack or perform model evaluation. Reports and
database files stay under that agent root. Only containers/network created by
that invocation are removed. Override `BUTLER_VERIFY_SUBNET` if its explicit
test subnet is occupied; do not prune other agents' Docker networks.

Baseline checked on `ssh laba`, 2026-10-15: key source files in
`agents/memory-search-review-fixes-20261015/current` match main `b20206d`.
The running orchestrator/Broker images still come from snapshot `3e26cd8`;
2026-10-15 restarts changed timeouts without rebuilding those images. Their
Doctor bridge omits a real run and prepare/authorize and discards ToolResult
status/error. Compare the actual image/source wiring rather than relying solely
on `current` or an old deployment manifest.
