# Issue #82: durable background memory execution

Implementation is ready for review. **Native success acceptance is blocked**:
the deployment's configured Antigravity `gemini-3.8-flash-tiered` returned HTTP 403
in the new isolated stand. No native result was published. This is not acceptance
of model quality or production enablement. References: #82, #77 and #81.

## Version and isolation

- Checked live issues #82/#77/#81 on 2026-10-15; all remained open.
- Base `origin/main`: `d98c93fcece405af672288a89bfe9d5b9ed303a1`, fetched again
  before committing. Local primary checkout and laba source checkout remained
  `d343485f550ce74cd1f46b3e02215169ba4cf295`; the primary dirty files were preserved.
- Main deployment remained `/srv/storage/butler/snapshots/20261014-main-3e26cd8/deploy`,
  image `sha256:ae02012c2637a92cc2a4f6c51e52f431eeccdc69d0c208cc28e8f23ef701960a`.
  Its actual read-only provider API, not an old acceptance report, confirmed
  Antigravity / `gemini-3.8-flash-tiered` active and connected.
- Isolated worktree branch `codex/issue-82-memory-jobs`; laba root
  `/srv/storage/butler/agents/issue82-20261015`, Compose project `butler-issue82`.
  Dedicated PostgreSQL 17, Redis, Orchestrator, Broker and proxy processes, private
  networks `10.253.82.0/28` and `10.253.83.0/28`, loopback HTTP 18282.
- Three separate databases: `memory_jobs_storage_test` with schema-per-test,
  `memory_jobs_legacy_test` for legacy public-schema fixtures, `memory_jobs_test`
  for native service execution. No main database writes or service switch.
- Production mechanism and main regression baseline: `d99f5b7`. Native-v1:
  `d1df2be` (only prospective window deadline changed). Final production code:
  `6f7c897` (safe failure diagnostics added). Native-v2: `a094a68` (only window
  script changed). Test-only `3b530db` adds the actual-wire crash scenario; later
  commits add this report/evidence without production code changes.
- Both full source manifests were checked byte-for-byte on laba. Source manifests,
  binaries, logs and receipts are in the [evidence directory][evidence]. No binary,
  private credential file, encrypted auth row or database dump is published there.

The required #81 mechanisms exist in main, independently of its issue status:
bootstrap registers durable metrics, passes the registry through
`orchestrator.Config.Metrics` to the memory bundle path, and composes the memory
Doctor reporter. Operator Doctor creates a current authorized task/input and uses
the actual Broker boundary, handling ToolResult errors. Its three assertions pass
against the separately built Broker in `validation-v10/doctor-broker.outcomes.json`.
This task extends that collector/reporter instead of replacing it. It does not
claim to close the entirety of #81 or to prove the old deployment has this wiring.

## Changed mechanism

`internal/memory/jobs` now admits immutable owner-authorized finite policies,
coalesces supported events, claims existing auxiliary queue rows, reserves the
existing shared execution budget, and executes through native model transport.
Its own current policy/task/source checks replace any reliance on an old run's
lease. Completed task status is permitted without keeping the task active.

Each possibly paid attempt has its own reservation/claim fence/source fingerprint
and known cloud locality. Usage is optional observation, never an inferred zero.
All conservative reservations remain charged after failure/restart; unused
capacity can be distinguished from observed usage but is not refunded by the
existing ledger contract. Catalog derivative creation and the unique result
receipt/job completion commit atomically. DATA output cannot grant permissions
or become an accepted profile fact automatically.

Global/policy pause, revoke and job cancellation remain durable and observed.
Queue 64, policy 128, max 32 jobs/policy, max 4 failure/crash claims, finite deadlines,
and shared owner/day/task/mandate budgets bound work. The worker/handler registry
defaults off/empty; only the explicitly enabled exact-source diagnostic probe was
used here. Legacy pipeline remains prohibited. #27/#28, #89/#90 and quality
promotion of extraction/compaction consumers are outside this change.

## Verification and criterion matrix

Automated Go test runs below used Go 1.27.2 on laba, real PostgreSQL 17 and `-race -count=1 -p 1`.
Counts are test/subtest events, not independent scenarios, and overlapping runs
are not added together.

| Run | Result | Meaning |
| --- | --- | --- |
| `validation-v10` |854 PASS,0 FAIL,2 SKIP|Jobs, budget, actionauth, Doctor, native adapter, catalog, owner API, orchestration, completion; separate actual Doctor/Broker test; vet and service builds completed.|
| `diagnostics-v11` |239 PASS,0 FAIL,0 SKIP|Final jobs/native-adapter regressions after safe diagnostics, including secret source and private provider error exclusion; vet and final Orchestrator build completed.|
| `wire-crash-v13` |2 PASS,0 FAIL,0 SKIP|Additional actual-HTTP crash subtest and its parent pass, using final production code plus the new test only; no cloud call.|
| `native-v1` |FAILED|One reserved attempt, no usage/result; the first version only stored `provider_failed`, so exact cause remains unknown.|
| `native-v2` |FAILED|One additional attempt, `provider_unavailable_http_403`, no usage/result. Same durable DB and owner/day spend retained.|

The two skips are `TestActualPostgresDumpRestoreDeletionIntegration` (external
pg_dump/restore opt-in unavailable in this Go runner) and
`TestBrowserOwnerStateFixtureServer` (interactive browser fixture). Neither is a
pass; full restore/browser acceptance is not claimed by this backend change.
An earlier v6 Doctor test failed because a missing table in a schema fell back to
native public tables via `search_path`. Its original failed log is retained.
Separating native/storage/legacy databases removed this fixture contamination;
v10 Doctor tests passed with the intended failure injection.

Tests listed here are searchable in each run's `*.outcomes.json`; full original
JSONL logs are retained as `*.jsonl.gz`. Controlled providers are explicitly
distinguished from native model access.

| #82 criterion / requested case | Current evidence | Limit |
| --- | --- | --- |
|Durable create, claim, crashes around dispatch and commit|`TestModelActualProcessCrashRecoveryIntegration` terminates a separate Go worker before/after the durable dispatch marker and kills it before commit against PG; the added `after_http_send` case proves SIGKILL after the parent HTTP server receives complete request bytes but before any response; `TestModelCrashRecoveryAndLostPublicationACKIntegration` retains each reservation and rejects old-fence publication.|Controlled transport; test alone accelerates the 2-minute expired claim.|
|Competing workers and expired claim|`TestModelConcurrentWorkersAndBudgetIntegration`; `TestModelDifferentJobsShareFinalBudgetUnitIntegration` races independent jobs for the last owner/day, root-task and mandate unit; `TestModelClaimExpiryAfterBudgetWaitIntegration` rejects expiry after lock wait.|Real transactional PG; no native concurrency spend.|
|Repeat events, lost publication ACK, one derivative|Crash/ACK test plus `TestModelBoundedTriggersQueueAndExpiredPendingIntegration`; same committed publication receipt is returned and one-result unique/CAS guards are exercised.|Native repetition/restart step was not reached because inference failed.|
|Budget exhaustion, unavailable provider, finite retry|Concurrent budget tests; `TestModelLocalOnlyUnavailableAndRetryBoundIntegration/unavailable` stops after 4 attempts; unknown usage remains NULL; `TestModelActualUsageOverrunRetainedIntegration` retains reported over-limit usage and denies publication; the original reservation remains charged.|Native HTTP 403 is non-retryable and was not automatically retried.|
|Pause/disable/cancel, worker shutdown recovery|`TestModelRevocationDuringCallIntegration`; `TestModelPolicyPauseKeepsJobsAndAttemptsIntegration`; `TestModelWorkerShutdownNormalizedTransportErrorIntegration` recovers from normalized native cancellation with both reservations intact.|Controlled in-flight transport.|
|Correction, deletion, source access revoke, task/mandate changes during call|Revocation test plus `TestModelMandateAndSourceRevocationDuringCallIntegration` and `TestModelSessionPrivacyRecheckedAtPublicationIntegration` reject every late output.|Real catalog/authority transactions with controlled callbacks.|
|Current background authority after terminal original run/task|`TestModelCompletedRunAndTaskPolicyIntegration`: original run/task completed, no lease, exact source → attempt/usage → atomic catalog publication; task remains completed.|Real PG, controlled inference; native completed-run success not established.|
|Interactive priority and independent user completion|`TestModelPriorityAtDispatchAndDuringInferenceIntegration`; `TestAuxiliaryMemoryFailureDoesNotUndoResultIntegration` passes with completion PG and Redis enabled.|Native foreground step was not reached; no successful native user run is claimed.|
|Local-only, no hidden fallback or raw credential release|Local-only tests make 0 provider calls; `TestModelTaskSourceCannotDropLocalOnlyBindingIntegration`; `TestModelCredentialSourceNeverReachesProviderIntegration`; safe diagnostic test excludes raw provider Message/Code/Details.|No local model selected. Recognizable-secret detection is an existing defense, not a proof of detecting every arbitrary secret.|
|Atomic rollback, transient persistence and retry|`TestModelTransientPublicationRollbackRecoveryIntegration` injects one SQL40001 after catalog insertion; final state has two charged attempts and one result. `TestMemoryJobActualNativeAdapterHTTPIntegration` uses actual native adapter/HTTP server and proves no HTTP on transient authority failure, then recovery.|Actual HTTP and PG; server responses controlled, not generative-model quality.|
|Own current admission, handler gates, bounded triggers/backpressure|Completed-run, consumer-gate, policy deadline, active-hour/privacy and queue tests; bounded source_changed/session_completed/session_inactive/schedule events.|Useful extraction/summary/consolidation handlers remain separately gated tasks.|
|Migration and safe rollback|`TestModelMigrationRollbackRetainsHistoryIntegration`: unused 071 down/up succeeds, occupied 071 down refuses and retains policy history.|Application rollback keeps additive 071 and all budget/attempt history.|

## New native windows and blocker

The prospective caps were written **before** calls. Both windows used the active
deployment provider/model and new neutral diagnostic sources. No historical
acceptance window, model answer, quota counter or success record was reused.
Native-v1 allowed at most 3 attempts / 90s each in 20min; its one unknown reservation
was retained. Native-v2 allowed at most 2 additional attempts in a fresh 20min
window, with initial-history count 1 and the **same owner/day cap 3**. The process
watchdog and EXIT cleanup disabled/stopped only the isolated Orchestrator.

At final read-only audit: 2 attempts, 2 failed, 0 published derivatives,
0 foreground attempts; both `usage=NULL`. Owner/day retains 2 turns,
2,113,536 reserved tokens, 180,000ms and 16,777,216 input bytes. Monetary quote 0
uses the explicit `subscription_no_overage` policy; subscription never proves
locality or zero token usage. The second attempt is durably classified cloud,
`provider_unavailable_http_403`. No third attempt, hidden fallback or model
switch was made. Control is disabled and isolated Orchestrator is exited.

Credential resolution succeeded, and the main provider API reports connected.
Those observations do **not** override the actual 403 or prove account entitlement.
The exact external cause of that rejection is not established. Restoring access
for the currently configured provider requires operator/upstream resolution;
changing credentials, account, provider or model is outside this implementation.

Consequently the full **real generative provider → validated response → atomic
publication + observed spend** success path and native foreground-while-background
scenario remain unconfirmed. Their deterministic mechanisms passed the real-PG
and actual-native-adapter/HTTP tests above. This PR must remain unaccepted for
native promotion until a newly authorized bounded window establishes success.
No summary/extraction/consolidation quality claim is made.

## Review, migration and deployment

An independent reviewer inspected implementation, source parity and original
receipts without performing inference. Fixed material findings: repeatable native
authority checks needed a separate single dispatch marker; normalized transport
errors must retain private transient SQL causes; worker shutdown could race with
a buffered native cancellation error. Corresponding regressions passed.
The reviewer separately checked v10/v11 outcomes and the native accounting.
See [review record][review] for remaining native acceptance limitations.

Apply 071 with the normal migrator. For application rollback: disable global
model control, stop worker, retain 071/history, run prior binary; old kind-specific
workers ignore model jobs. Down is supported only for an unused schema. Do not
delete reservations/history or enable the legacy pipeline to force rollback.
See the [full contract and controls](../architecture/background-memory-jobs.md).

Reproduction scripts are under `scripts/dev/memory-jobs-*.sh`. Native-v1/v2 labels
are consumed and must not be replayed or used to replenish spend. Main deployment
was not switched; PR merge and automatic issue closure are not part of this task.

[evidence]: ../../testdata/acceptance/issue82-20261015/
[review]: ../../testdata/acceptance/issue82-20261015/review.md
