# Issue #82: durable background memory execution

**Bounded native acceptance passed on laba**, using the existing Antigravity
Gemini Flash High subscription profile through the isolated proxy. Three new
calls produced two verified atomic background publications and one foreground
completion while local-only work waited. Nine earlier charged attempts remain
unchanged. This proves the general execution mechanism, not consumer/model
quality or production enablement. References: #82, #77 and #81.

The old AICode/tiered path still returned403 after OAuth renewal and on both
network lanes. Expiry was not established as its cause. High was explicitly
selected before the successful window; no automatic provider/model fallback
was added, and the primary deployment's model/configuration was not switched.

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
  for native service execution. No main application data/schema or service switch. The later
  owner-requested auth repair renewed only the encrypted main provider-auth row;
  its encrypted backup remains private. No credential material is published.
- Production mechanism and main regression baseline: `d99f5b7`. Native-v1:
  `d1df2be` (only prospective window deadline changed). Initial diagnostic production code:
  `6f7c897` (safe failure diagnostics added). Native-v2: `a094a68` (only window
  script changed). Test-only `3b530db` adds the actual-wire crash scenario; `f5939e7` strengthens
  observed signal exit and reservation/scope assertions. Later
  commits initially added this report/evidence. Follow-up core diagnostics:
  `a6c8529`, `c4e5efb`, `4fb6913`; accepted binary SHA256
  `7c32853646ccc48adc0e4930f8c68d3d0814b9a70df8f49d848ad1588019b2ec`.
  Successful prefix script: `dc7a441`; same-window remainder and Broker/proxy
  configuration repair: `9b25bfa`. The experimental tiered Cloud Code option
  was removed in `eb3c0f8`; it is not part of the resulting transport contract.
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
| `crash-v14` |5 PASS,0 FAIL,0 SKIP|All four crash cases and parent; forced child signal exit and two distinct reservations / three charged scopes explicitly asserted. No cloud call.|
| `native-v1` |FAILED|One reserved attempt, no usage/result; the first version only stored `provider_failed`, so exact cause remains unknown.|
| `native-v2` |FAILED|One additional attempt, `provider_unavailable_http_403`, no usage/result. Same durable DB and owner/day spend retained.|
| `recovery-v20` |303 PASS,0 FAIL,1 SKIP|Auth/quota/fixed401/403 diagnostics and jobs/native transport regressions; missing provider-auth PG opt-in was a skip, not a pass.|
| `authpg-v21` |1 PASS,0 FAIL,0 SKIP|The skipped encrypted-PG import/reconnect test explicitly enabled and passed against isolated PostgreSQL.|
| `release-v27` |338 PASS,0 FAIL,0 test SKIP|Resulting jobs, config, provider auth, native transport, private CLI; race/vet/build. providerfactory has no test files, reported separately as package skip.|
| `diagnosis-v30` |196 PASS,0 FAIL,0 SKIP|Final transport/rejection-envelope/private CLI tests after safe FAILED_PRECONDITION and access projections; race/vet and both service/operator builds.|
| `native-v10` + remainder |PASS, bounded scenario|Three actual High/proxy calls; exact quote publications, repeated event/restart, foreground marker, completed-task background authority, retained old spend; SQL ledger assertions pass.|

The v13 wire test did not yet assert the Kill/Wait result; it is retained as
historical wire/recovery evidence. The stronger v14 run proves forced signal
termination and retained reservations/scopes explicitly.

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
|Repeat events, lost publication ACK, one derivative|Crash/ACK test plus `TestModelBoundedTriggersQueueAndExpiredPendingIntegration`; same committed publication receipt is returned and one-result unique/CAS guards are exercised.|Native-v10 repeat/restart retains exactly one job, attempt and result; lost-ACK failure injection remains controlled.|
|Budget exhaustion, unavailable provider, finite retry|Concurrent budget tests; `TestModelLocalOnlyUnavailableAndRetryBoundIntegration/unavailable` stops after 4 attempts; unknown usage remains NULL; `TestModelActualUsageOverrunRetainedIntegration` retains reported over-limit usage and denies publication; the original reservation remains charged.|Native HTTP 403 is non-retryable and was not automatically retried.|
|Pause/disable/cancel, worker shutdown recovery|`TestModelRevocationDuringCallIntegration`; `TestModelPolicyPauseKeepsJobsAndAttemptsIntegration`; `TestModelWorkerShutdownNormalizedTransportErrorIntegration` recovers from normalized native cancellation with both reservations intact.|Controlled in-flight transport.|
|Correction, deletion, source access revoke, task/mandate changes during call|Revocation test plus `TestModelMandateAndSourceRevocationDuringCallIntegration` and `TestModelSessionPrivacyRecheckedAtPublicationIntegration` reject every late output.|Real catalog/authority transactions with controlled callbacks.|
|Current background authority after terminal original run/task|`TestModelCompletedRunAndTaskPolicyIntegration`: original run/task completed, no lease, exact source → attempt/usage → atomic catalog publication; task remains completed.|Native-v10 remainder admits after owner-verified task completion, without a lease; task stays completed.|
|Interactive priority and independent user completion|`TestModelPriorityAtDispatchAndDuringInferenceIntegration`; `TestAuxiliaryMemoryFailureDoesNotUndoResultIntegration` passes with completion PG and Redis enabled.|Native foreground exact marker and observed usage complete while the local-only job remains pending with zero attempts.|
|Local-only, no hidden fallback or raw credential release|Local-only tests make 0 provider calls; `TestModelTaskSourceCannotDropLocalOnlyBindingIntegration`; `TestModelCredentialSourceNeverReachesProviderIntegration`; safe diagnostic test excludes raw provider Message/Code/Details.|No local model selected. Recognizable-secret detection is an existing defense, not a proof of detecting every arbitrary secret.|
|Atomic rollback, transient persistence and retry|`TestModelTransientPublicationRollbackRecoveryIntegration` injects one SQL40001 after catalog insertion; final state has two charged attempts and one result. `TestMemoryJobActualNativeAdapterHTTPIntegration` uses actual native adapter/HTTP server and proves no HTTP on transient authority failure, then recovery.|Actual HTTP and PG; server responses controlled, not generative-model quality.|
|Own current admission, handler gates, bounded triggers/backpressure|Completed-run, consumer-gate, policy deadline, active-hour/privacy and queue tests; bounded source_changed/session_completed/session_inactive/schedule events.|Useful extraction/summary/consolidation handlers remain separately gated tasks.|
|Migration and safe rollback|`TestModelMigrationRollbackRetainsHistoryIntegration`: unused 071 down/up succeeds, occupied 071 down refuses and retains policy history.|Application rollback keeps additive 071 and all budget/attempt history.|

## Fresh native recovery and preserved accounting

All windows wrote prospective caps before inference, used the same isolated DB
and owner, stopped on failure, and retained all reservations. No historical
acceptance answer or closed window is reused as current proof.

| Window | Model / lane | New admitted attempts | Observed result |
| --- | --- | --- | --- |
| v1/v2 | tiered / proxy |1 each|Unknown first failure; then HTTP403. Both usage NULL.|
| v3 | tiered / proxy, renewed OAuth |1|HTTP403 PERMISSION_DENIED despite unexpired renewed auth.|
| v4 | experimental tiered Cloud Code / proxy |1|Paid discovery timed out before generation; reservation conservatively retained.|
| v5 | experimental tiered Cloud Code / direct |1|HTTP400; compatibility option rejected and later removed.|
| v6 | original tiered AICode / direct |1|Same HTTP403 PERMISSION_DENIED; proxy alone does not explain it.|
| v7/v8 | existing High / direct |1 each|HTTP400; exact earlier causes remain unknown.|
| v9 | existing High / direct, improved diagnostics |1|HTTP400 with canonical FAILED_PRECONDITION. Does not establish which prerequisite failed.|
| v10 prefix + remainder | existing High / isolated proxy |3 total|Two BG publications and one FG answer, all with observed usage; original bounds and old history retained.|

The actual main auth record was expired. Normal Manager.Resolve renewed it with
OAuth HTTP200 and persisted encrypted rotation, then copied that encrypted record
privately into the stand. The earlier isolated403 occurred before its own expiry,
and v3 still returned403 with renewed auth. Paid discovery and model metadata
returned200, but never substituted for a generation result. Read-only access
metadata confirmed paid tier/project presence; absent consent/onboarding fields
remain null. No terms were accepted and no account/provider/credits were changed.

The artificial stand-only owner/day cap3 was removed at the owner's explicit
request by restoring the existing default Day allowance on that same fixture row.
The before/after receipt in budget-reconfiguration-v19 proves the same scope ID,
used2 and original reservations. No runtime top-up/reset mechanism was added;
normal immutable shared budgets and finite per-window/policy/root limits remain.

Native-v10 was frozen before calls to High/proxy, at most3 new calls,90s each,
15min total and cumulative12 including old9. Prefix admission started17:12:58Z;
original expiry17:27:57Z. It published the exact quote, then reposted the event
and restarted the service: one job/attempt/result persisted. Local-only policy
waited with zero attempts.

The first foreground input stopped before model admission because gRPC sent
internal tool-broker traffic through HTTPS_PROXY: NO_PROXY omitted that hostname.
The correction adds internal service names; actual ListTools then succeeds in the
service's exact environment, with zero tool/model execution. The old blocked input
is preserved. The owner resume surface created a continuation for that same task;
no SQL task/run status was fabricated. Remainder permits only the two unspent
calls and reads the original deadline, without extension/refund/replaying BG1.
It completed at17:17:54Z. Prefix remains an interrupted receipt; the combined
bounded scenario and remainder have an explicit PASS.

| Actual call | Saved usage | Verified effect |
| --- | --- | --- |
| BG1 |297 input,85 output,777 total|Exact source quote; current source parent/revision, one atomic derivative.|
| Foreground |6264 input,9 output|Exact ISSUE82_FOREGROUND_READY; owner verification/completion through real task controls.|
| BG2 after task completion |297 input,85 output,644 total|Task completed17:17:46Z, new admission17:17:47Z, publication17:17:53Z; no active original lease.|

Final ledger:11 BG attempts plus1 foreground,12 distinct reservations,2 published
sources,9 old unknown usage records unchanged (row/reservation digests match).
Observed input/output subtotal is6858/179; unknown usage is not included as zero.
Foreground total is not recorded by its owning attempt table and is not invented.
Owner/day conservatively retains12 turns,12,681,216 quoted tokens,1,080,000ms and
100,663,296 input bytes. Shared task root uses2/2 (FG+BG2). No refund was applied.
Cost quote0 uses explicit subscription_no_overage; all generation is cloud, and
membership never proves local execution or zero actual token consumption.

Same-profile/proxy quota observations:0.99958575 before and0.9992839 after,
reset2026-10-15T21:19:22Z. These are server model metadata, not per-request
billing attribution or proof that all account activity was ours. The final auth
expiry remained17:31:10Z. Main image/path are unchanged; only its explicitly
requested encrypted auth renewal wrote to main. Native worker control is disabled,
service exited; final audit has no active input/run/lease and local attempts0.

High/proxy is the newly proved operational configuration for this account.
The old AICode403 and direct High prerequisite are not declared repaired or
causally explained by token expiry. Production activation must explicitly select
the proved profile and matching budget price basis; no fallback is installed.
The exact-source diagnostic probe is the only admitted handler; extraction,
summary/consolidation quality and broader model behavior remain separate gates.

## Review, migration and deployment

An independent reviewer inspected implementation, source parity and original
receipts without performing inference. Fixed material findings: repeatable native
authority checks needed a separate single dispatch marker; normalized transport
errors must retain private transient SQL causes; worker shutdown could race with
a buffered native cancellation error. Corresponding regressions passed.
The reviewer separately checked original v10/v11 outcomes, final v27/v30 tests and native-v10/remainder accounting.
See [review record][review] for independent observations and the High/proxy/diagnostic-handler limits.

Apply 071 with the normal migrator. For application rollback: disable global
model control, stop worker, retain 071/history, run prior binary; old kind-specific
workers ignore model jobs. Down is supported only for an unused schema. Do not
delete reservations/history or enable the legacy pipeline to force rollback.
See the [full contract and controls](../architecture/background-memory-jobs.md).

Reproduction scripts are under `scripts/dev/memory-jobs-*.sh`. Native-v1–v10 labels
are consumed and must not be replayed or used to replenish spend. Main deployment
was not switched; PR merge and automatic issue closure are not part of this task.

[evidence]: ../../testdata/acceptance/issue82-20261015/
[review]: ../../testdata/acceptance/issue82-20261015/review.md
