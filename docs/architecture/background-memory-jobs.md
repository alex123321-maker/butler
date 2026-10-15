# Bounded generative memory jobs (#82)

The Orchestrator extends `internal/memory/jobs` and `internal/budget`; PostgreSQL
owns policy, queue, claims, attempts, reservations and results. The existing
worker loop performs a bounded scan, without a separate scheduler or Redis queue.
The old extraction/summary pipeline remains disabled.

## Authority and triggers

An authenticated owner creates an immutable, finite `ModelPolicyRequest` through
`POST /api/v4/memory/model-policies`. It binds the versioned code handler, exact
original source IDs/access revisions and ancestry, optional original run/root
task, current task/constraints/assignment revisions, explicit privacy, allowance,
deadline (at most seven days), interval (60–86400 seconds) and maximum jobs (1–32).
The policy is separate current background authority. It never uses session leases
or changes task status. A completed root task is allowed under its **current**
revision. Completion itself changes that revision: a policy created before this
transition cannot silently rebase and requires a new explicit owner policy.
Delegated tasks are denied by this composition; no child scope is broadened.

Supported events are `source_changed`, `session_completed`, `session_inactive`
and `schedule`. The scanner considers at most 32 policies per tick. Events bind
the complete current source manifest; session events additionally require the
original completed run and no active run in that session. Inactive sessions need
the configured quiet interval. Schedule intervals use UTC time slots and do not
replay missed slots. Equal events coalesce into the same stable job identity.
Only source content revisions may advance; access changes or changed ancestry
cannot inherit permission. New work still has its own exact immutable manifest.

At most 128 unexpired active/paused policies and 64 pending/running model jobs can
exist globally. Serialized admission enforces these bounds under concurrent
producers. Full queues retain a visible `queue_full` policy reason; they create
no unbounded jobs. MaxJobs and the policy's shared allowance bound lifetime work.
Interactive Session Service ownership suppresses scheduling/admission/HTTP and
preempts in-flight auxiliary work within the 250ms authority supervisor interval.

Task/assignment permissions remain current even after the original run ends.
Task sources cannot omit or substitute their task binding. Session history needs
its matching original run and cannot combine unrelated task privacy policies.
Session locks precede tasks and block conflicting FK task insertions during a
dispatch/publication transaction. All ancestral sources need current access.
Secret-class data and recognizable raw credentials deny processing.

## Attempts, transport and publication

1. Claim the existing `memory_aux_jobs` row with its monotonically increasing
   epoch and at most two-minute claim. Four actual failure/crash claims are allowed.
2. Check current task/assignment, global control, policy and full catalog manifest.
   Lock order is sessions → tasks/authority → control/policy → sorted sources →
   job → sorted budget scopes. No transaction remains open during provider HTTP.
3. `ModelQuote` uses the configured provider's full `InferenceLimits` and the
   operator output ceiling. Atomically reserve job, policy, root-task, assigned
   mandate and owner UTC-day allowance through the existing execution ledger.
   Persist the attempt/reservation, claim fence, source fingerprint, provider,
   model and independently known locality before calling transport.
4. Recheck DB time and all authority after budget waits. Clamp the call to every
   scope deadline, policy, claim and current goal/mandate active-hours window.
   A fresh transport correlation ID is neither a user run nor a lease.
5. Use the configured native provider with no tools, fresh provider session,
   typed DATA provenance, complete serialized input-byte ceiling and
   `WithInferenceAuthority`. The actual native HTTP hook commits a possibly-sent
   marker immediately before HTTP. This marker is conservative, not a receipt
   proving upstream execution. No retry/fallback/account rotation is hidden here.
6. Save valid complete observed usage before validation/error/publication. Absent
   usage remains SQL NULL; over-allowance usage is retained and denies publication.
   The journal contains hashes/metadata, never response/source text or secrets.
7. The code-owned handler validates its response. Recheck source revisions/access,
   task/mandate/privacy, policy, claim and clock in the publication transaction.
   Create one immutable catalog DATA derivative with all parent dependencies,
   then commit its pointer, attempt outcome and job `done` atomically. It is not
   an accepted profile fact, task state or action permission. Catalog correction,
   revoke and privacy deletion apply to that result through existing dependencies.

Crash before send, after send or before commit preserves the reservation. A new
claim creates a new attempt and never resends a paid operation ID. Lost publish
acknowledgement resolves to the committed opaque result receipt; the worker does
not infer again. A one-result unique index and claim CAS prevent duplicate
derivatives. Transient storage/connection/serialization failures retry within
the original limits; an uncertain commit first reconciles durable publication.
Provider failures retry only when the transport marks them retryable, at 10-second
intervals. Exhausted budgets, unknown pricing and permanent invalid/stale source
or authority do not create endless retries.

Conservative reserved capacity is retained for **all** attempts, including known
short calls, unknown outcomes and cancellation before HTTP. This preserves the
existing ledger's no-refund contract. Actual usage and unused reserved capacity
remain distinct across restart; no crash or policy revision replenishes counters.
Subscription pricing declares monetary overage policy, never locality or usage.

## Controls, composition and observability

`BUTLER_MEMORY_MODEL_JOBS_ENABLED=true` composes the worker. It defaults false.
The PostgreSQL global control separately starts `disabled`; an owner-proved
`POST /api/v4/memory/model-worker-control` selects `enabled`, `paused` or `disabled`.
Process flags require restart; control state and allowance survive restart.
`POST .../model-policy-control` selects `active`, `paused` or irreversible
`revoked`; `POST .../model-cancel` cancels one pending/running job. Cancellation,
revocation and disabling reject late results, including a provider ignoring cancel.
Pause before a network dispatch does not consume the model retry count. A real
interrupted dispatch still counts; its reservation remains charged.

`GET /api/v4/memory/model-jobs` returns owner-scoped policies/jobs, current reasons,
attempt states, conservative quotes, optional actual usage and result references.
Failed provider attempts retain only the transport's allowlisted diagnostic code
and optional numeric HTTP status. Raw messages, codes and provider details never
enter this observation; `provider_failed` remains the job's bounded reason.
All mutations require the existing owner bearer and delivery identity; these are
not model tools. Doctor's durable job observation adds global control, attempts,
unknown-usage count and reserved tokens/cost. The existing content-free collector
exports these without owner/job/source metric labels. Configured worker/control,
observed outcomes and assistant/model quality remain separate facts.

The initial generative adapter accepts the current native Antigravity provider,
whose complete limits, serializer ceiling and immediate HTTP authority hook are
verified. Other adapters fail closed until these boundaries are supported.
`local_only` waits as `local_backend_unavailable` without a cloud reservation or
fallback; this work neither selects local models nor resumes #27/#28.

The handler registry defaults empty. `BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED`
explicitly enables only `source-quote-v1`, a diagnostic exact-source quote checker.
It proves transport/plumbing with bounded neutral DATA and establishes no summary,
extraction, consolidation or answer quality. Consumers from #84/#85/#89/#90 need
their own quality/security gates; none is silently enabled by #82.

## Migration and rollback

Apply migration 071 before starting the new binary, using the normal migrator.
It extends the existing auxiliary queue and references the existing reservation
ledger/catalog. No dependency or extra durable service is introduced.

Before application rollback, disable global control, pause/revoke desired
policies and stop the model worker. Keep migration 071 and all budget/attempt
history when running the previous application; old kind-specific workers ignore
`model` jobs. Do not enable the legacy pipeline. Migration 071 down refuses any
policy/job/attempt history. An unused schema can be rolled down normally; deleting
history to force rollback is not supported. Preserve the catalog deletion anchor
and the original immutable budget policies in backup/restore.

## Acceptance

Use `deploy/memory-jobs/compose.yml` in a dedicated `ISSUE82_ROOT` on `ssh laba`.
Real PostgreSQL tests create separate schemas with MaxConns=2. Process-crash tests
kill an independent Go worker inside publication and accelerate only the test
claim expiry. Controlled transport failures are distinct from native inference.
The optional `native.compose.yml` uses separate Orchestrator/Broker/Redis/PG/proxy
processes and one fresh bounded diagnostic window. Private encrypted provider
credentials and owner/service tokens are not published with evidence.

The [per-criterion acceptance report](../testing/issue-82-memory-jobs.md) records
the passing controlled tests, failed native windows, retained costs and remaining
quality boundaries. Historical windows cannot establish this version.
