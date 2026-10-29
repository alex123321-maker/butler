# Browser authority and restart qualification

Additional #163 / PR #177 qualification after the complete assisted login.
Previous matrix slots, native failures and timings remain unchanged. This window
separates zero-inference integration fault injection from a new native Core
restart trial. Integration durations are test-process time, not a model speed
benchmark. Native Input wall time includes deliberation, context/queue/operator
waits, restart and tool work, with delivery acknowledgement reported separately.

## Actual Broker/PostgreSQL/Chromium matrix

The test uses current production Broker code in a separate authenticated OS
process, actual PostgreSQL stores and a real Chromium runtime subprocess. It
supplies test-only task/context/source wiring on an isolated `_test` database;
this does not qualify the complete owner-UI cancellation workflow. The current
task state and real mandate/grant/effect/resource/budget records live in PG.
No production database, model, owner credentials or native matrix action is used.

| Case | Recipient POSTs | Required observation |
| --- | ---: | --- |
| unchanged control | 1 | The same permitted path actually reaches Chromium and the recipient; opaque acknowledgement remains UNKNOWN. |
| task cancelled after authorization | 0 | Original grant remains unused; zero click dispatches/reservations; opening cost remains. |
| mandate revoked after authorization | 0 | Actual PG mandate revision/revocation denies dispatch; same retained grant/cost checks. |
| changed expected origin | 0 | The original grant cannot authorize a different origin. |
| changed selector/args | 0 | The original grant cannot authorize even a different selector on the same page. |
| Broker killed after opaque ACK | 1 | An actual frozen read does not promote UNKNOWN; a fresh Broker process cannot replay or send a successor POST; all three reservations remain. |
| Broker killed while response is held | 1 | Receiver acceptance precedes process kill. Owning MarkInterruptedEffect records UNKNOWN after the executor is dead; fresh Broker rejects replay/successor; both reservations remain. |

The last case invokes the existing owning interruption method explicitly; it
does not claim automatic crash classification by production bootstrap. In both
restart cases the original authorization and single dispatch identity remain.
Neither a new process nor a snapshot is an external receipt. These cases cover
Broker crash/reconstruction, not restoration of a lost Chromium DOM or a native
model takeover after a cancelled task.

Final race-instrumented Broker matrix: **seven cases and two parent tests passed,
zero skips/failures**. Actual Chromium was used throughout; vet passed. Test
container wall time was 7.630s (including process startup/exit); parent test
elapsed times were 3.63s and 1.93s. No model was invoked. This is not native task
latency and contains no missing Butler model-thinking interval.

Proof root:
`/srv/storage/butler/agents/browser-safety-20261029/reports/20261029T052220Z-1187368`.
The source is commit 55959c6d plus the separately hashed new test file. Raw-safe
[test log](browser-baseline/safety/integration-broker-chromium.txt),
[phase timings](browser-baseline/safety/integration-timings.jsonl),
[source hashes](browser-baseline/safety/integration-source.sha256),
[manifest](browser-baseline/safety/integration-manifest.txt) and
[exit receipt](browser-baseline/safety/integration-exit.json) are retained.

The required credential-vault CI now runs both parents and checks every named
subcase without skips. `verify-changed.ps1` identifies this requirement. Future
regressions must retain the positive control alongside the denial cases.

## Failed qualification and operational limits

The first launcher had a missing shell line continuation; no test ran. Earlier
fixture windows blocked all positive submissions: Go sniffed the bare `<form>`
body as text/plain, so Chromium could not find the button. Those windows are
failures, not negative acceptance evidence. The fixture now explicitly serves
HTML; a separate positive-control window passed before the full seven-case run.
All earlier reports and source versions remain in the operator root.

The shared host was short of storage and the operator hit its existing /tmp
user quota. Go build temporary files used a bounded private container tmpfs.
Five retired test executables were preserved as gzip archives with verified
decompression hashes, retaining sources, reports and production binaries. The
initial unprivileged compression failed before removing any original. Protected
owner-control temporary files for the new native trial moved to its private
operator directory; no owner token is exported or retained in evidence. No
quota, budget, PIN or policy reset occurred.

## New native Core restart trial

`core-restart-01` is an additional ordinary form request, outside the original
matrix. Its fresh HTTPS receiver recorded exactly one accepted quantity-3 form
at 05:29:39.85829841Z. The original click became UNKNOWN and the kernel recorded
a frozen observation plus a pending factual owner review.

After confirming that this was the only active Input, the operator restarted
the actual existing Core container at 05:30:24.704442879Z. At
05:30:35.823886176Z it was healthy, on the same binary and private environment;
Broker/browser/PG/Redis/Web containers were unchanged. The same Input/run still
waited for that review, with unchanged tool transcript and model usage. It had
not published or dispatched another POST. A subsequent independent check also
compared the complete before/after usage projection for equality.

Only then did the operator check the receiver again and send an explicit factual
report through the protected owner API. This is assistance, not an automatic
oracle, and preserves the original UNKNOWN transcript/grant/spending. The same
Input/run resumed, closed the browser and corrected one locally rejected
communication draft before publishing its result:
`publication:106849a22f9368dd75c17ee55c10be04367cf490d227e2c25aa6071315e0f163`.
Protected owner GET verified title, requested quantity and saved wording; no
credential canary or uncertainty wording appeared in that bounded text check.
The receiver still had exactly one accepted quantity-3 POST. Web recorded
`result_ready`; Telegram recorded `sent` (API acknowledgement, not human read).

Full Input wall time was **456.194s**, including deliberation, all operator/queue
waits and restart: rounded inter-tool gaps 426.208s, tools 17.439s, finalization
12.548s. Independently rounded subintervals can differ by 1ms from the rounded
total; the unrounded CSV partition residual is effectively zero. Telegram sent
at 447.802s overlaps that interval. There were 14 recorded tool/local-validation
spans and 14 model attempts, 146,397 input / 1,399 output tokens, no unknown usage
pairs. Monetary cost and isolated model-only thinking time remain unavailable.

Assistance was four exact action approvals (one preparation failed before input
and was reassessed) plus one factual report. The additional Core restart was an
explicit test-operator fault injection. No previous scenario was rerun to fill
timings or replace a failure. See the
[process check](browser-baseline/safety/core-restart-01-process-check.json),
[final native metadata](browser-baseline/safety/core-restart-01-metadata-final.json),
[private comparison](browser-baseline/safety/core-restart-01-verification.json)
and [inclusive action timings](browser-baseline/safety/timings-inclusive-v1/README.md).

General browser acceptance and #163 remain open after this qualification;
owner-UI cancellation/revocation, runtime-loss boundaries and integration/merge
status must not be inferred from a component test or a single assisted trial.
