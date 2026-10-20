# Issue 17 current main local acceptance

Date: 2026-10-20. Source baseline: `fbd4954102abc811be91f9c36ead544eab908576`.
Final verdict: pending the complete local receipt.

The supported runtime is Linux inside Docker Desktop on the owner's Windows PC.
No SSH, remote lab execution, model inference or personal browser account is used.
PostgreSQL, real files/private RPC, installed production native host and actual
Chromium establish kernel behavior; scripted proposals do not establish model
quality or external recipient verification. The primary checkout is preserved.

## Required criterion matrix

| Issue criterion | Owning code and required current-source evidence |
| --- | --- |
| 1. One executor per tab; owner intervention advances observation | `internal/resources`, `internal/resourceexec`, production Chromium extension; actual `two_executors`, `manual_input` and `manual_new_observation` scenarios. |
| 2. Stale page plan cannot execute | Exact current selection/host/origin and synchronous extension observation/CAS; actual `dom_mutation`, `navigation`, `foreign_host`, `foreign_origin` and stale manual-observation scenarios. |
| 3. Independent code copies and conflict before overwrite | `TestConcurrentRealPatchesCannotOverwriteAcrossStoreInstancesIntegration`, `TestResourceWorkspaceIndependentLeaseContendersIntegration`, actual Broker stale-patch-before-spend test. |
| 4. Finite leases, ordered acquisition and defined crash recovery | Resource set atomicity/opposite order, actual contended deadline, real runtime process loss/SIGKILL and authenticated discard with independent physical absence. Browser UNKNOWN stays quarantined. |
| 5. Expired owner cannot write | `TestExpiredOrSupersededAuthorityCannotWriteAfterWorkspaceWaitIntegration`, current resource/task/parent fences, post-claim expiry and stale input/source denial at the actual runtime/Broker boundary. |
| 6. No authority/secret transfer via workspace temps | `TestIsolatedCopiesScopesAndPrivateTempsIntegration`, wrong owner/scope/root and private RPC denial, private root inode ownership and source graph checks; no working credentials mounted. |
| 7. Atomic whole-task budgets | `TestChildAndToolReservationsShareAtomicParentLimitsIntegration`, `TestSharedReservationsSurviveRestartAndRejectParallelOverspendIntegration`, child root-budget reset denial, Broker conflict rolling back the whole admission. |
| 8. Worker facts remain candidates; head changes use CAS | Actual Broker `TestMemoryCandidateDispatchedIntegration`, lost ACK/dedup expiry and `TestExtractionConcurrentWorkerCandidatesNeedOwnerHeadCASIntegration`; one competing owner decision wins and the other becomes stale. |

## Reproduction

```powershell
pwsh -NoProfile -File scripts/dev/issue17-local.ps1
```

The entrypoint uses the existing Go 1.27.2, pgvector PostgreSQL 17 and Playwright
1.52.0 pins. Its isolated internal test network publishes no host ports. Fixture
private roots, sockets, tokens and browser profiles live inside Linux container
temporary directories. Existing local services and databases are not touched.
The fixed production Unix socket parent is provisioned on container-private
tmpfs with mode0700 and UID/GID10001, matching the non-root runtime requirement.
Dependencies/image preparation uses the network; acceptance has no internet route.

Every selected test must run; any test skip/failure denies the final marker.
Command-only Go packages with no tests are not counted as passes. The two actual
browser runtime entrypoints are separate: this issue's guarded user-tab Chromium
test is mandatory, while controlled-server-browser e2e belongs to issue8/web-tool
acceptance and is not selected here. Private subprocess harness entrypoints run
through their parent integration tests rather than as skipped standalone tests.

## Evidence

### Live lease fixture correction

The original negative recovery case selected a 1500ms accepted-input deadline
before normal action admission, then assumed its resource lease was still live
after physical failure/journal preparation. The local PostgreSQL diagnostic
measured **-0.047 seconds remaining before RequestDiscard** on the failing create
case: accepting that already-expired lease was correct behavior. The original
failure and diagnostic remain retained separately.

Only this negative test now selects a finite 10s input deadline before normal
admission. It proves at least 5s remaining, requires the exact recovery-conflict
error while the lease is still live, and checks unchanged UNKNOWN/version/fence/
spending with no published recovery intent. It then waits for actual PostgreSQL
expiry and proves physical discard/replay behavior. No production timeout,
resource expiry, clock or authority check was changed. Other fixtures retain
their 1500ms input deadline. Both corrected create/write subcases passed under
UID10001 with race detection in the focused regression.

### Full receipt

The final run records source SHA256 before/after, source commit/diff, container
versions, stage JSONL, actual Chromium log, binary hashes, vet and a complete
marker under a unique repository-local `.cache/issue17/` directory. A compact
content-free receipt is added after the final run. No partial run closes the issue.

Personal-account/browser installation, network-policy activation and production
rollout remain separate operator actions. Same-tab UNKNOWN reconciliation is
deliberately unavailable; expiry/restart never proves effect absence or replays
a click. Workspace discard retires the copy and retains original UNKNOWN/spending.
Native Windows runtime ACL support and model semantic quality are outside these
Linux resource-arbitration criteria.
