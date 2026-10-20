# Issue 17 current main local acceptance

Date: 2026-10-20. Source baseline: `fbd4954102abc811be91f9c36ead544eab908576`.
Final verdict: **PASS for all eight issue17 criteria**.

The complete frozen run tested `1d37d40910200504948b4f91206e687f91bff6fc` with
an empty working diff. Production Go, configuration, schemas and browser code
match the current-main baseline; the only changed Go file is the recovery test
fixture described below. This report and its [content-free JSON receipt](fixtures/issue17/current-main-local-20261020.json)
were finalized after the run without changing executable code or test selection.

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

The shared discard fixture now selects a finite 10s input deadline before normal
admission. A later full run also exposed three setup failures with the original
1500ms deadline: no physical pending checkpoint had been reached. Focused repeats
did not reproduce those failures; their original failed full run remains retained.
The longer finite fixture deadline allows the real runtime to reach the injected
publication fault before expiry. Missing physical state still fails with dispatch
phase/error class and remaining-lease diagnostics, never a retry or skip.

The negative recovery test proves at least 5s remaining, requires the exact recovery-conflict
error while the lease is still live, and checks unchanged UNKNOWN/version/fence/
spending with no published recovery intent. It then waits for actual PostgreSQL
expiry and proves physical discard/replay behavior. No production timeout,
resource expiry, clock or authority check was changed. The workspace package has
a 10-minute test ceiling to accommodate all natural expiry waits. Both corrected
create/write subcases passed under UID10001 with race detection in the initial
focused regression; the final shared fixture also passed the complete run.
The final shared fixture also passed the focused queue, lost-ACK, root-replacement
and live-lease create/write regressions with race detection under a one-CPU,
UID10001 container before the complete run.

### Full receipt

Run: `.cache/issue17/20261020T141536Z-f0db000e`. The final entrypoint exited0
and wrote `complete=PASS` only after every mandatory gate. It ran **146 top-level
Go tests and 209 subtests with race detection**, plus all **10 actual Chromium
scenarios**. There were **zero failed or skipped tests**. The command-only
`apps/browser-bridge` package has no tests and is excluded from these counts.
Targeted `go vet` passed. All19 specifically required integration tests passed.

| Stage | Top-level passes | Subtest passes |
| --- | ---: | ---: |
| Resources and shared budgets | 33 | 11 |
| Memory head CAS | 3 | 0 |
| Child task control/budgets | 4 | 0 |
| Guarded tab control/native protocol | 24 | 39 |
| Broker resource/workspace admission | 21 | 16 |
| Actual workspace and guarded runtime bootstrap | 3 | 21 |
| Core owning context/candidate dispatch | 5 | 32 |
| Owner HTTP controls | 3 | 0 |
| Core guarded request suspension | 1 | 3 |
| Workspace files, provenance, crash and owner recovery | 49 | 87 |

Actual Chromium passed `valid`, `manual_input`, `manual_new_observation`,
`dom_mutation`, `navigation`, `foreign_origin`, `foreign_host`, `lost_ack_restart`,
`two_executors` and `installed_native`. The last case verifies Chrome's actual
installed native manifest launch; the tests also assert real local recipient
call counts, finite reservations and retained UNKNOWN/quarantine behavior.

The source manifests before and after the run have the same SHA256:
`799eb9df123e53f3a661cb78bb7a2f02460a52e91a434e43548175f5c4c53063`.
The JSON receipt retains exact image IDs, binary/log hashes, stage test names and
the five earlier incomplete/failed attempts. Raw logs, source manifests, source
commit/diff, Chromium output and vet remain in the local run directory. Earlier
attempts never received a final PASS and are not substituted for this run.

One earlier full attempt exposed missing Unix socket provisioning in the new
local runner: TCP bootstrap passed, Unix bootstrap denied its absent private
parent. The runner now provisions an isolated UID10001/mode0700 tmpfs parent;
both real transports passed. Runtime permission checks were unchanged. Only
this invocation's temporary containers, network and executable volume were
removed; the existing local PostgreSQL/Redis and primary checkout were preserved.

Personal-account/browser installation, network-policy activation and production
rollout remain separate operator actions. Same-tab UNKNOWN reconciliation is
deliberately unavailable; expiry/restart never proves effect absence or replays
a click. Workspace discard retires the copy and retains original UNKNOWN/spending.
Native Windows runtime ACL support and model semantic quality are outside these
Linux resource-arbitration criteria.
