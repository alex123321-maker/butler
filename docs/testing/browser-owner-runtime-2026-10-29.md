# Owner UI and browser runtime loss qualification

Additional #163 / PR #177 acceptance after the assisted login and Core restart.
No original baseline slot was replayed, replaced or retrospectively timed.
Production services, credentials, PINs, quotas and browser policies were unchanged.

## Owner UI joined to actual dispatch

Real Chromium opened the existing Butler Web controls and pressed **Stop task**
and **Revoke mandate** after a browser click had already been authorized. The
requests went through production owner HTTP handlers, Task Controller / Goal
Manager and disposable PostgreSQL with the full ordered migrations. A separate
authenticated Broker process read the resulting authority and attempted the
original grant against the real server-browser runtime.

| Case | Canonical evidence | Recipient POSTs |
| --- | --- | ---: |
| Stop task | One task-control receipt; UI task version 2/cancelled; original grant unused and still unexpired; zero dispatches | 0 |
| Revoke mandate | One owner-authority receipt/version 2; old authority denied before asynchronous branch propagation; original grant unused and still unexpired; zero dispatches | 0 |
| Unchanged positive control | Same actual API/PG/Broker/runtime path successfully dispatches; opaque acknowledgement remains UNKNOWN | 1 |

The owner endpoints and responses are real. Only unrelated navigation uses the
existing `openMindPanel` bootstrap fixtures. Authority/context/source composition
is test-only: the loopback adapter projects actual PG task, lease and assignment
state and calls the canonical assignment checks; it cannot mint approvals. This
is not a native model run or proof that every production Core composition works.
All records, owner proof and site data are synthetic; browser storage did not
contain the synthetic token.

The lab reused the existing built Web snapshot from
`server-browser-owners-20261029/current`. Its `TaskOwnerControls.vue` and
`advanced/goals.vue` source hashes match this checkout (see manifest). CI builds
the complete current Web source afresh. The new owner-review UI was not deployed.

Final test with the explicit unexpired-grant guard passed: one Playwright test,
both negative decisions and one positive control; race-instrumented Broker/API;
no skips. Proof root:
`/srv/storage/butler/agents/browser-safety-20261029/reports/owner-ui-20261029T061116Z-1406103`.
[Broker evidence](browser-baseline/owner-runtime/owner-broker-expiry-guard.txt),
[browser evidence](browser-baseline/owner-runtime/owner-browser-expiry-guard.txt),
[timings](browser-baseline/owner-runtime/owner-expiry-timings.jsonl) and
[hashes](browser-baseline/owner-runtime/owner-expiry-proof.sha256) are retained.
The earlier positive-control run remains alongside it; no failed result was erased.

## Actual browser runtime loss

The new tests kill and wait for the real Go browser-runtime process, then start
a fresh process on the same private endpoint. This is not a fake executor failure
or only a Broker restart. Chromium executes the fixture through its normal
guarded runtime and private resource authority. No tool bypasses Broker.

| Loss point | Required result | Recipient POSTs |
| --- | --- | ---: |
| Read context exists, no pending write | Old-context read cannot complete; explicit fresh open succeeds with a different ID | 0 |
| Runtime killed before submitting the authorized click | Conservative UNKNOWN and original spending/dispatch identity retained; replay and successor denied after restart | 0 |
| Site accepts POST while response is held, then runtime is killed | UNKNOWN and the two reservations retained; replay and successor denied after restart | 1 |

The two write-loss cases retain one original dispatch and its authorization.
The resource wrapper intentionally hides the backend's detailed session error;
the stale-context check does not claim that it exposed a `session_lost` code.
A new browser process does not restore DOM or resolve the old external effect.

The complete Broker matrix now has **10 cases and 3 parent tests passed, zero
skips/failures** under race instrumentation, plus the separate owner-UI test.
Broker and API vet passed. Proof root:
`/srv/storage/butler/agents/browser-safety-20261029/reports/20261029T060326Z-1371194`.
See [matrix log](browser-baseline/owner-runtime/runtime-matrix.txt) and
[phase timings](browser-baseline/owner-runtime/runtime-timings.jsonl).

Because the shared resource fixture gained explicit runtime start/stop and
external-PG composition, all 10 existing `TestResource*Integration` parents also
ran under race instrumentation: all passed, zero skips, including the two real
Chromium cases. Container wall time was 5.060s. The
[regression log](browser-baseline/owner-runtime/resource-regression.txt) and
[timings](browser-baseline/owner-runtime/resource-regression-timings.jsonl) are
retained; the required credential CI now includes this fixture regression too.

## Inclusive time and operational failures

From the first recorded operator timestamp **05:44:51 UTC** to final acceptance
at **06:11:28.196971353 UTC**, work took **1,597.197s (26m37.197s)**. This includes
Codex deliberation, inspection, implementation, failed setup/build attempts,
waiting, fault injection and verification. It includes the stricter final grant
expiry check. It is an operator work window, not a native Butler Input latency;
no Butler provider/model ran in these fixtures. Time before the first recorded
timestamp is unavailable. Documentation/CI completion after this acceptance
boundary is tracked separately.

| Inclusive operator phase | Seconds |
| --- | ---: |
| Initial runtime qualification, including setup and reasoning | 485.211 |
| Initial joined owner UI qualification | 452.866 |
| Positive control and fresh-context qualification | 189.771 |
| CI/documentation setup and stricter unexpired-grant qualification | 469.350 |

These are consecutive wall-clock intervals, not model CPU time or reconstructed
per-token thinking. The original first acceptance window is preserved separately.
See [inclusive work window](browser-baseline/owner-runtime/inclusive-work-window-final.json).

Final automated UI durations were **2.609s** for Stop task and **2.612s** for Revoke
mandate, including panel navigation, interaction and observed acknowledgement.
The UI test container took 9.200s; the separate full Broker/runtime matrix took
10.190s. The two combined qualification containers overlapped; these durations
must not be added to the inclusive work window or described as model speed.

Several preparations failed on the pre-existing /tmp user quota / filesystem
capacity: source transfer, hard-link staging and executable copy. Failed windows
were not acceptance evidence. New code was re-transferred and hashes checked;
build scratch used bounded container tmpfs. Owned retired test executables were
gzip-preserved with verified decompression hashes. Final test binaries are under
`/dev/shm/butler-owner-final-bin-20261029`; older binary archives are under
`/dev/shm/butler-owner-retained-20261029`. These are volatile lab artifacts, not
production releases. Source, logs and hashes are preserved separately. No broad
prune, production log deletion, quota reset or production container restart occurred.

## Required regression checks and remaining scope

`credential-vault-tests.sh` requires all 10 named cases without skips. The
credential acceptance workflow additionally builds current Web and runs
`browser-owner-safety-tests.sh` for the joined owner UI path, retaining both
server and browser logs. Ordinary Web-only jobs explicitly skip this opt-in case;
that skip is not acceptance evidence.

The first CI run on e3be72e0 passed the required credential/runtime/resource
step and ordinary Web typecheck/lint/e2e, but the new owner UI launcher failed:
it ran the Broker executable from the repository root (relative registry path
unavailable), and Web's independently pinned Chromium revision had not been
installed. The launcher now uses the Broker package directory; CI explicitly
installs Web's own browser revision. No package versions or production contracts
changed. The failed [CI run and artifacts](https://github.com/alex123321-maker/butler/actions/runs/40828133657)
remain evidence of that setup failure, not owner-control acceptance.

The next [CI window](https://github.com/alex123321-maker/butler/actions/runs/40828862790)
passed both actual owner decisions and the one-POST positive control, but the
current app's background action-outcome polling exposed an early API teardown.
A late route fetch received connection refused after the API had exited. The
fixture now waits for Playwright to drain its routes and close the page before
the final acknowledgement permits API shutdown. Request errors are not swallowed
and control/receipt assertions are unchanged. That failed window is retained;
successful control assertions alone are not a successful end-to-end test.

These bounded checks pass. General browser acceptance and #163 remain open:
they do not establish universal browser capability, autonomous reconciliation,
native model recovery after runtime loss, popup SSO, CAPTCHA or WebSocket support.
No PR merge or whole-branch lab rollout is implied.

Follow-up: [native runtime recovery](browser-native-runtime-recovery-2026-10-29.md)
separately qualifies a real pre-input browser-runtime SIGKILL through the same
Input/run, fresh context, one POST, privately checked answer and Web/TG delivery.
It counts explicit owner assistance and retains all seven additional attempts;
the integration-only scope and original matrix above are unchanged.
