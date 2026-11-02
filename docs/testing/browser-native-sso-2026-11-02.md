# Native bounded SSO with response delivery

Historical qualification before the remaining #164 gates were completed; see
[native06 discovery/delivery](browser-sso-discovery-2026-11-03.md) and
[external Google04](browser-native-google-2026-11-02.md) for later evidence.
This qualifies a synthetic native-form popup SSO flow,
not arbitrary sites, JS-only forms, OTP or Google. The natural-language goal
included public fixture selectors and URLs. The model independently discovered
tools, selected the saved account, obtained one protected consent, invoked SSO
and published its answer. It was not given the private reference or fields.

The actual pipeline was Antigravity `gemini-3.8-flash-high` → Core → Broker →
PostgreSQL/resource authority → server Chromium → two public HTTPS origins →
Web/Telegram. The origins were `butler-sso-rp.report-template.ru` and
`butler-sso-idp.report-template.ru`; TLS validation remained enabled. Dedicated
nginx blocks permit only the lab NAT and loopback. DNS, certificate and recipient
on `ssh devops` are separate from the existing baseline service.

## Exact successful attempt

Event `browser-baseline-sso-native-20261102-04`,
Input `input-263be397e514443349e9d60b5b5f1716`,
Run `run-1793612415676629140-747b14ed3cae2191`.
Core used `fd1c99c3`; Broker/browser used `28b76c50`; recipient used `2e6735b0`.
One finite owner goal/mandate admitted this new Task; old grants were not reused.

The [read-only private verification](browser-baseline/native-sso-20261102/native-sso-04-verification.json)
confirms one SSO request, one protected approval, one immutable use claim, both
field releases and **zero generic approvals**. The user explicitly delegated
this synthetic consent to the test operator. The PIN was entered in the owner
UI only; approval is counted as operator assistance, not autonomous consent.

The [recipient journal](browser-baseline/native-sso-20261102/recipient-04.jsonl)
records one accepted IdP login, one accepted callback and two authenticated
account reads (browser and independent Go probe). Canonical action evidence is
`effect_confirmed`, `dispatch_completed`, `browser_sso_login_v1`, with
`submitted_once`, `authenticated_read` and `anonymous_challenge` all true.
The synthetic recipient was stopped after verification at 09:53:17 UTC. Its
original JSONL SHA256 is
`d27073236276b5789a96853c3aa7b5aff45c53b71e2deeb042b5fc0f03f1309d`;
the exported journal is byte-identical.
The private reference in the protected Web publication matched the independent
fixture oracle. No username/password literal occurred in this Run's retained
tool args/results, messages or publication. Provider wire/system logs were not
scanned; no broader absence claim is made.

Web persisted `result_ready`; Telegram persisted `sent` (API acknowledgement,
not human read). The saved answer was also visibly inspected in the owner UI.
Input/Run completed; durable Task completion remains unverified and the owner
card still reports that separately. This test does not manufacture a final Task
verification or close #164.

## Inclusive action timing

Ingress: **09:40:15.469692 UTC**. Web availability: **09:45:00.700333 UTC**.
Telegram ACK: **09:45:01.712602 UTC**. Total to delivered response:
**286.243 seconds (4m46.243s)**, including model deliberation.
Input terminal bookkeeping completed at 09:45:11.515614 UTC (296.046 seconds).

Protected request → approval was **83.057 seconds**. It includes operator UI
interaction and diagnosis, not just human typing. Excluding that overlapping
wait gives 203.186 seconds to Telegram. There were **11 model attempts**,
126,000 input tokens and 1,932 output tokens; monetary cost is unknown.

Each inclusive action interval starts at the preceding recorded tool completion
(first interval starts at ingress) and ends at this tool completion. Thus the
model's deliberation, scheduling/context and other gaps are retained. These
gaps cannot be identified as pure provider inference time.

| Action | Inclusive interval, s | Recorded tool span, s |
| --- | ---: | ---: |
| Discover tools 1 | 23.100 | 0.240 |
| Discover tools 2 | 5.414 | 0.119 |
| Discover tools 3 | 5.573 | 0.227 |
| Discover tools 4 | 6.084 | 0.185 |
| Load tools | 11.913 | 0.476 |
| Find saved account | 10.900 | 0.152 |
| Open RP page | 6.729 | 0.688 |
| Consent, planning and verified SSO | 183.596 | 1.950 |
| Prepare/publish response | 32.149 | 0.381 |

The SSO interval includes the 83.057-second approval wait; do not add it twice.
The 1.950-second executor span is not the whole agent login latency.
[Exact timings](browser-baseline/native-sso-20261102/native-sso-04-final-timings.json)
and [model/delivery metadata](browser-baseline/native-sso-20261102/native-sso-04-final-metadata.json)
were derived from existing durable records without action or model replay.

## Retained unsuccessful attempts

| Trial | Input wall time, s | Outcome |
| --- | ---: | --- |
| 01 | 596.551 | RP browser binding expired during owner wait; UNKNOWN, no recipient login |
| 02 | 132.305 | Prior unresolved attempt correctly blocked new write authority |
| 03 | 999.743 | No approval before expiry; zero use claims and no SSO submit |
| 04 | 296.046 | Completed Input, verified login and delivered response |

Trial01 was resolved separately through the protected historical owner outcome
API after context expiry/settling, terminal Run and inactive lease checks. A
[counted operator attestation](browser-baseline/native-sso-20261102/native-sso-01-owner-attestation.json)
used the sealed empty synthetic journal after stopping that recipient.
This was not automatic production verification. Its failed Input/Run, UNKNOWN
tool transcript, consumed claim and spending remain retained. Trial04 received
new scope, authority and consent; no old POST was replayed or grant refunded.

Fixes: RP binding survives a ten-minute protected consent wait (15-minute
binding; form/code TTL remains three minutes). A late-arriving SSO card now
initializes its pinned account while the panel is already unlocked. Selecting
another account remains disabled; polling never approves. Seven real Chromium
vault UI tests passed, including late arrival and incomplete-scope rejection.
An unrelated CI teardown EOF was traced to owner harness shutdown before its
final HTTP response flushed; graceful shutdown preserves the final receipt.

Historical outcome review and policy regressions additionally passed
[193 targeted PostgreSQL/race tests and subtests](browser-baseline/native-sso-20261102/recovery-regression-summary.json),
with no failures or skips. Linux SSO execution at `fd1c99c3` passed
[required CI](https://github.com/alex123321-maker/butler/actions/runs/41413708240).
The EOF fix at `eac35ad3` passed
[vault/real-owner-UI CI](https://github.com/alex123321-maker/butler/actions/runs/41422958136).
The late-card fix passed seven local Chromium cases and Vue typecheck.
Main remains `065a7d55`; its three triggered post-merge workflows
([Web](https://github.com/alex123321-maker/butler/actions/runs/41393374663),
[fixture](https://github.com/alex123321-maker/butler/actions/runs/41393374678),
[vault](https://github.com/alex123321-maker/butler/actions/runs/41393374671)) passed.

## Lab operations and remaining acceptance

Old test-build cleanup removed six unreferenced builder images and 18 unmounted,
unused historical ELF binaries (633,022,422 bytes). All container bind mounts,
including stopped containers, were protected. No volumes, databases, reports,
private configs, active/rollback images or unrelated issue115 services were
removed. At 09:50 UTC `/` had 5.1 GiB available and `/srv/storage` 493 MiB.

Lab-only deployment retained private snapshots, original mounts, database dump
and rollback configuration under `/home/laba/butler-sso-20261102`. Core-only
historical recovery and Web-only polling fix passed health, unchanged environment
and unchanged peer-container checks. Web polling fix uses `5e7cd6b6`.
No merge/main deployment follows from this qualification.

At this historical stage, remaining checks included broader hostile/runtime-loss composition,
independent model page selection beyond supplied fixture parameters, external
Google smoke and separate durable Task completion. Fixture success does not
establish those capabilities. Engineering, cleanup and CI wall time is separate
from the measured native login slot.
