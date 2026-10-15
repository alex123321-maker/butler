# Issue 81 implementation and bounded acceptance

Date: 2026-10-15. Branch: `codex/memory-observability-81`.
Base: `b20206d9df5b91e06702aed1ee7bf44a144e3ade`.

## Baseline and scope

The fresh source snapshot was located on `ssh laba`, at
`/srv/storage/butler/agents/memory-search-review-fixes-20261015/current`.
Git object hashes of memory service, semantic hybrid reader, HTTP composition
and action authorization specification matched the main base above.

The running Compose containers still reference deployment snapshot `3e26cd8`.
Their orchestrator/Broker image digests and start times were checked before and
after this task and remained unchanged. A separate 2026-10-15 timeout deployment
record states that its service restarts did not rebuild the images. The old
Doctor source bridge supplies neither a real run nor prepare/authorize and
discards ToolResult status/error; the old bundle composition supplies Metrics nil.

Implementation and tests used a separate managed worktree and isolated agent
root. No production pipeline, deployment or model evaluation was enabled.
The primary checkout and its existing changes were retained.

## Result

- The ordinary Orchestrator context path shares its `/metrics` registry with
  memory service. Search, optional reads, context selection and whole-bundle
  completion have independent outcome/duration observations. Counts and bounded
  reasons accompany SelectionDiagnostics without query or memory text. Existing
  selected-class hit/miss counters retain their meaning.
- Empty, no-match and unknown have separate semantics. Zero document hits alone
  remain unknown. Backend failures, cancellation, denial, stale/revoked sources,
  context allowance and execution admission have bounded typed reasons. SQL
  fallback retains its result and separate semantic limitations.
- Read-only PostgreSQL observations expose durable jobs, retry/failure reasons,
  pending age, index build/verification/activation and saved generation proof.
  Scrapes recover gauges from storage after restart and omit missing values
  rather than manufacturing zeros. Legacy Redis queue state stays separate.
- Owner-authenticated Doctor admits a real task/session/input and uses the
  existing worker, lease, Broker prepare/authorize/dispatch and approval recovery.
  Its fixed local command never calls a provider. Pending reports survive the
  Approvals navigation; final reports are stored once and history is owner scoped.
- Runtime/RPC/result/reporting errors preserve independent checks. Unknown,
  missing or stale observations cannot become a full healthy report. Semantic
  composition is independent of legacy pipeline enablement. Existing generation
  evidence is displayed separately from unverified assistant answer quality.
- API, Doctor UI, architecture specs, service READMEs and the
  [operator runbook](../operations/memory-observability.md) were updated.

## Verification

Final isolated source snapshot:
`/srv/storage/butler/agents/memory-observability-81-20261015/snapshots/20261015-202335-633`.

Final proof directory:
`/srv/storage/butler/agents/memory-observability-81-20261015/reports/memory-observability-20261015T132341Z-2779072`.

`complete` contains `PASS`; `diagnostics.log` is empty. Go test JSON records
**524 passed test/subtest events, 0 failures, 37 skips**, across memory
diagnostics/Doctor/service/semantic, metrics, API, durable input, Orchestrator and
the selected app composition test. These numbers include subtests, not 524
independent end-to-end scenarios. Race detection and targeted go vet passed.
The skipped conditions remain skips; they do not establish native model,
external-account, unrelated storage-fixture or production readiness.

The new required composition/storage tests ran with actual isolated PostgreSQL:

- `TestOperatorDoctorActualBrokerIntegration` runs an actual Broker process
  against the real private context reader and a controlled gRPC runtime. Mode 2
  dispatches once; mode 1 dispatches only after an exact owner approval and worker
  restart. A missing grant does not reach the runtime. A nil model provider
  proves that the fixed command and its recovery require no model interaction.
- `TestDurableMemoryObservationsRecoverAndFailWithoutZeros` recreates readers
  and registries over persisted jobs/generations, checks owner isolation and
  validates that unavailable storage emits failed observation rather than zero.
- `TestDoctorStorageKeepsHistoryScopedAndOneReportPerRequest` tests concurrent
  final polling and owner-scoped historical compatibility.
- `TestDoctorPendingReportSurvivesApprovalNavigation` retains the pending receipt
  and advances the same row to an immutable final observation.
- `TestOrdinaryRunUsesCompositionMemoryRegistry` exercises the normal run/context
  path with controlled stores/provider and observes its metrics handler.
  Separate tests cover SQL fallback limitations, all outcome rules, private
  metric cardinality, partial reporter failure and ToolResult error handling.

Local copies of the four JSONL logs, PASS marker and summary are in
`.cache/acceptance/memory-observability-81-20261015/` in the implementation
worktree. The test script removes only its own container/network and preserves
proof files and the isolated database data under the agent root.

Web typecheck, targeted Doctor stylelint and **2 Playwright tests passed**. Browser
tests use controlled API responses: historical compatibility, pending receipt
reload, exact-approval presentation, partial memory/runtime failures, unknown
queue depth and stale owner-proof response rejection. They are separate from
the actual PostgreSQL/Broker composition test. Screenshot:
`web/test-results/doctor-observability.png` in the worktree.

A broad app-package run on Windows also encountered existing native private-file
permission and frozen historical-fixture failures. It is not recorded as a pass.
Targeted app composition, including the real Broker, passed on Linux. The current
ESLint configuration ignores the modified legacy page/composable paths; typecheck,
stylelint and executable browser coverage were used explicitly instead.

## Remaining boundaries

Component SQL health, generation verification and assistant quality are separate.
There is no accepted, configuration-bound assistant quality reference in the
current runtime, so Doctor reports it unverified; #78 is not imported as a
production verdict. Configured inference/worker objects retain unknown liveness
without a real observation. The declared signal thresholds are provisional
operator diagnostics; #91 measures limits and #92 establishes system readiness.

This task does not enable legacy pipeline workers, add workers, change retrieval
ranking, rerun a model benchmark or deploy the implementation. The dedicated
Doctor run records an observation; generic task completion remains subject to
the normal independent verification contract.
