# Native browser execution with a bounded owner mandate, 2026-10-30

This qualification uses ordinary protected Web ingress, a real model worker,
Tool Broker, PostgreSQL, Chromium, a separate HTTPS synthetic recipient and
the existing owner-goal/mandate contract. Issue #163 remains open; the general
browser baseline is NO_GO. The public form requires no credential consent.

## Implementation and checks

- Protected Web ingress accepts an exact active owner Task in its existing
  conversation. Task identity is included in immutable ingress/replay identity.
- An explicitly listed `communication.message` can use the fixed operator-pinned
  reply resource. Destination admission and export/source checks remain separate.
- Owner-goal preparation reads the complete current Task transcript, including
  previous attempts and interaction responses, rather than unrelated session
  history. Other Task kinds and context allowances remain unchanged.
- ID-only browser reads bind to the current owned context URL and control epoch.
  The Broker checks the returned final URL and current context before releasing
  bytes. Cross-owner/run/Task contexts and changed epochs are denied.
- Recovery resolves the canonical owning interaction before active-only source
  readers, then rechecks those sources before model/tool execution. Current
  authority, versions, evidence, claims and source denials remain authoritative.

The exact code commit `68b5fd6e7b9ad5bd0fd2eb096fee7960c87ad2b3` passed all
11 CI checks, including the mandatory PostgreSQL owner-outcome recovery test.
Earlier targeted full/narrow builds, race tests and 13 real Broker/Chromium
mandate cases are retained under [evidence](browser-baseline/native-owner-mandate/).
The injected foreign-result case is a component release-guard test, not a native
foreign navigation. Component provider-boundary tests are not native delivery.

## Retained attempts

Each accepted Input below is separate. None was replayed to collect timing.
Initial CLI Task-ID validation failed before HTTP admission; the corrected
driver admitted attempt 01 once. Setup also retains a rejected reused delivery
identity before mandate 04 was created with a fresh identity.

| Attempt | Input wall time, s | Model attempts | Result |
| --- | ---: | ---: | --- |
| 01 | 50.164 | 0 | Shared session context exceeded the allowance; zero POSTs |
| 02 | 40.070 | 2 | Conservative mandate active-time reservation exhausted; zero POSTs |
| 03 | 17.725 | 1 | Conservative mandate token reservation exhausted; zero POSTs |
| 04 | 62.424 | 6 | ID-only snapshot lacked the owned URL scope; zero POSTs |
| 05 | 166.887 | 10 | One approved POST accepted, owner report confirmed, source-order recovery failed; no result delivery |
| 06 | 187.201 | 13 | Complete native form, same-Input outcome recovery, browser read and Web/Telegram answer delivery |

Attempts 01–05 retained the original goal deadline, 2026-10-29 18:42:41 UTC.
New bounded mandates replaced revoked predecessors without resetting their
usage. The 24-turn adapter bound is 1,048,576 input plus 32,768 output tokens per
turn: 25,952,256 reserved tokens. This is not measured token consumption.
Operator Task/day budgets and provider/context limits were not changed.

Attempt 05 observed invalid quantity 8 locally, then valid quantity 3 in a
frozen form, with zero local-fill approvals. Exactly one protected click approval
dispatched one accepted quantity-3 POST. One separate owner-assisted factual
report confirmed the independent recipient journal. The original click transcript
remains UNKNOWN, and the failed Input/run has no result publication. The report
does not constitute successful recovery or machine-receipt verification.

## Operational and timing boundaries

The operator work window began 2026-10-29 17:42:32 UTC. All wall-clock figures
include model deliberation, preparation, scheduling and approvals/waits. Action
CSV intervals run from the preceding tool completion (Input acceptance for the
first action) through the current tool completion. Gaps are not pure model latency.
Publication delivery acknowledgement is not a human-read receipt. Monetary cost
is unknown; retained accounting records report known tokens and unknown usage.

The first narrow rollout failed due to a Broker executable basename mismatch;
its first rollback used an incomplete Compose file list. Explicit original
mount/environment overlays repaired that staging error. Core v27 and Core/Broker
v30 were subsequently verified with unchanged environments and strict click
approval. Only bounded test overlays were deployed; no migrations ran here.

`laba` stopped serving SSH/Butler around 18:40 UTC while still answering ping.
The user rebooted it around 18:53 UTC; service access returned around 18:54 UTC.
The outage cause is unproven. Durable attempts/effects/reports survived; RAM-backed
build trees did not. No failed Input was revived. The Core v30 source was
reconstructed and reproduced its installed SHA-256 exactly before applying the
recovery fix. A fresh red/green test reproduced the active-only source-reader
failure and passed the full seven-case recovery matrix. A stale narrow-build
test call signature was then aligned with the already-committed Web API tests;
this changes no production binary behavior. Build containers use finite memory
and temporary cache limits. Outage, reconstruction and verification time remain
part of the operator window.

## Complete native attempt 06

[Joined qualification](browser-baseline/native-owner-mandate/native-mandate-06-qualification.json)
records Input `input-b5e16b6976557b9b06e58d9f3e721165` and run
`run-1793301401549732704-b1d22237d31acdf4`. A separate goal and finite mandate
expired at 20:01:21 UTC; this did not extend or reset attempts 01–05.
The exact HTTPS origin, nine listed methods, read/prepare/unknown effects,
24-turn/40-call caps and no initiative/credentials bounded the attempt. The
persisted typed mandate omits the false initiative field; its default is false.
The initial 15-minute CLI observation argument was refused before HTTP admission;
the same prepared Task was then admitted once using the existing 10-minute bound.

Three completed local fills have three mandate-backed grants with empty approval
IDs. One approved click dispatched one accepted quantity-3 POST. One separate
factual report confirmed its independent recipient evidence. The original UNKNOWN
click transcript remains immutable, while the action-effect journal records the
report's resolution. The same Input/run resumed, read the independently generated
reference using `browser.read`, published one correct result and closed its browser.
No HTTP fetch substituted for that browser read. Private fixture values and raw
answer text were not exported.

Protected Web retrieval verified the reference, title, quantity and reported
validation/correction. Web is `result_ready`; Telegram has a persisted `sent` API
acknowledgement at **177.688s** from Input acceptance. Input/run completed at
**187.201s**. There were 13 model attempts, 135,090 known input and 1,564 known
output tokens, with zero unknown usage pairs. This independently qualifies the
bounded scenario; it does not mark the product Task criteria verified or the
owner goal completed. The Task was still active before test-authority cleanup.

The initial read verifier used the recipient snapshot taken before outcome
reporting, so its read count was still zero. That comparison is retained. A fresh
read-only export of the final journal contains exactly one form submission and
one read; both [final form](browser-baseline/native-owner-mandate/native-mandate-06-form-final-verification.json)
and [final reference](browser-baseline/native-owner-mandate/native-mandate-06-read-final-verification.json)
comparisons pass for the same result publication. No browser/model action was
repeated for this evidence refresh. The form verifier's `expected_answer_present`
is deliberately false because that scenario has no expected text token; the
reference scenario performs the actual private answer comparison.

This is **zero preparation approvals and one POST approval**, plus **one separate
factual outcome report**. Creating goal, mandate and Task adds three protected
owner mutations, for five owner operations in this first-task example. No claim
of one total owner interaction or autonomous machine-receipt recovery is made.

## Inclusive action timing

[Per-action CSV](browser-baseline/native-owner-mandate/inclusive-timings/actions.csv),
[all six trials](browser-baseline/native-owner-mandate/inclusive-timings/trials.csv)
and [method](browser-baseline/native-owner-mandate/inclusive-timings/README.md)
derive exclusively from retained records. CSV numbers use invariant decimal
format. All six Input partitions are complete; floating-point residual is below
0.001ms. Failed attempts and failed/UNKNOWN calls remain in the exports.

| Attempt 06 interval ending in | Inclusive seconds |
| --- | ---: |
| Two catalog searches and tool loading | 49.382 |
| Open form | 12.741 |
| Snapshot | 5.799 |
| Fill title | 4.660 |
| Invalid quantity 8 | 4.818 |
| Correct quantity 3 | 5.946 |
| Click, including submit approval wait | 36.269 |
| Freeze snapshot | 0.095 |
| Read reference, including factual-report wait and recovery | 38.393 |
| Publish answer | 18.284 |
| Close browser | 4.297 |
| Finalization after the last tool | 6.517 |
| **Input total** | **187.201** |

Recorded tool spans total only 3.488s; the remaining 183.713s include deliberation,
context, scheduling, owner waits and finalization. They cannot be labelled pure
model inference time. The operator qualification/cleanup window through the final
restore audit is **17:42:32–19:22:13 UTC, 99m41s**, including investigation, thinking,
builds, failed attempts, outage, user reboot and requalification. Documentation and
final CI review continue afterward and are reported separately; they are not
hidden inside the 187.201s native result.

## Cleanup and limits

[Authority cleanup](browser-baseline/native-owner-mandate/authority-cleanup.json)
confirms both test goals and all five test mandates revoked before restoring
the original registry. Failed Input 05 and its one dispatch remain; no budgets
or history were deleted. [Restoration](browser-baseline/native-owner-mandate/baseline-restored.json)
verified healthy Core/Broker, original mounts/environments, running binary hashes
and complete original registry hash. Core v32 and strict Broker v30 qualified the
scenario; the lab now runs its pre-test Core v17/Broker v13 baseline. The separate
HTTPS service returned to `evidence-reuse-01`; the new recipient journal is retained.
No merge or general production rollout is claimed.

This proves one synthetic public-form scenario on Antigravity
`gemini-3.8-flash-high`, including owner-assisted UNKNOWN resolution and delivery.
It is not broad website, CAPTCHA, popup-only SSO, login/OTP or WebSocket acceptance,
nor repeated-trial reliability or a performance SLO. Issue #163 remains open and
the overall browser baseline remains NO_GO.
