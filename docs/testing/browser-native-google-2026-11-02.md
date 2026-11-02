# External Google native test, 2026-11-02

**Latest result: Google04 PASS at57719e6a** for the bounded external diagnostic
flow, including completed same Input and actual Web/Telegram result delivery.
The native-form independent-page-selection criterion remains separate.

Google04 used one protected owner decision, one launch, one exchange and no
generic POST approval. The RP authenticated/anonymous differential and its child
effect were verified; one result publication belongs to the original Input and
contains the retained correct reference. Web `result_ready` occurred
16:24:03.850685 UTC (**288.145166 s** from ingress), Telegram `sent`
16:24:05.154572 (**289.449053 s**). Input finalized16:24:21.958676
(306.253157 s). Its10 model attempts consumed90233 input /1019 output tokens.

| Google04 inclusive phase | Seconds |
| --- | ---: |
| Ingress → protected request | 125.227206 |
| Request → operator consent | 60.789626 |
| Consent → browser handoff | 0.839528 |
| Handoff → exchange claim | 51.639187 |
| Provider exchange + RP probes | 2.029941 |
| Verified fact → Web result ready | 47.619678 |
| Web ready → Telegram accepted | 1.303887 |
| **Ingress → Telegram accepted** | **289.449053** |

All model deliberation, scheduling, native actions and operator waits are inside
these phase intervals. Only the two-second provider/probe phase is mechanical;
it is not the agent's login speed. Google04's pre-opened window worked directly,
with no ACL repair or tab relocation during that attempt. The counted operator
unlocked the test vault, approved one request and selected the dedicated
already-signed-in Google account. No new Google consent screen, password or MFA
was required. It does not prove password/MFA automation or Chrome-session import.

The owner Web UI subsequently rendered exactly one result card with the exact
check URL and privately matched RP reference. Its history was manually expanded
to include the fresh publication; rendering was checked16:29:15.805 UTC, not
measured as first-paint latency. Server `result_ready` and visible UI verification
are distinct observations. Actual Telegram acknowledgement is the native timing
endpoint above. The private OAuth client secret and dedicated email were absent
from the scanned native payloads; provider wire/all system logs remain outside
that scan. Safe machine evidence is
[native04-proof.json](evidence/browser-native-google-2026-11-02/native04-proof.json)
and [native04-ui-proof.json](evidence/browser-native-google-2026-11-02/native04-ui-proof.json).

## Retained earlier failure

At d50001f5 on laba, native Google03 completed real Google authentication and
the independent RP access check. **Same-Input result delivery failed**, so this
attempt is not complete #164 acceptance or successful ingress-to-delivery timing.
The earlier attempts remain retained and were not replayed for metrics.

One protected owner decision, one launch and one exchange claim were retained.
The callback verified Google's signed identity for the selected dedicated
account. The exact RP authenticated read and anonymous negative control both
passed; its child effect is CONFIRMED. No generic POST approval was created.
The failed message proposal contained the correct retained RP reference but an
unselected `message:` source reference; there was no publication or delivery.
Its dispatched metadata outcome remains UNKNOWN pending separate owning review.

| Inclusive native phase | Seconds |
| --- | ---: |
| Ingress → protected request creation | 107.095417 |
| Request → operator consent | 46.027144 |
| Consent → browser handoff start | 122.159211 |
| Handoff → provider exchange claim | 66.811628 |
| Exchange + independent RP probes | 1.985001 |
| Verified fact → failed Input finalization | 102.007164 |
| **Ingress → failed Input** | **446.085565** |

Ingress was15:51:19.906473 UTC; verified access15:57:03.984874; failed Input
15:58:45.992038. Every interval includes model deliberation, scheduling and
intervening operator/network waits. These are durable milestone intervals,
not pure tool runtimes. The11 model attempts had observed usage109252 input /
1421 output tokens. Native Google02 previously failed before consent after
108.293923 seconds,6 attempts and46594 /694 tokens; exact authorization rejected
its external-login proposal. No provider exchange occurred in02.

The counted operator entered the authorized test vault PIN, approved one
request, selected the dedicated Google account and continued the identity/email
screen. An existing signed-in Chrome session was used: this proves neither
vault password injection nor MFA automation. Chrome's initial new handoff
window received the test RP's nginx403; the launch had not been consumed.
After a root-only address probe, the same unspent link was opened in the
already reachable RP tab. Only observed test-browser egress addresses were
allowed, no owner/API routes were exposed, and callback query logging remained
disabled. This assistance and infrastructure diagnosis are inside the timings.

An operator scan of this run's checkpoint/model inputs, tool args/results,
messages and publications found neither the actual private OAuth client secret
nor the dedicated Google email. Raw payloads/reference/provider queries were
not exported. Provider wire and every system log were not audited by that scan.

Private lab evidence:
`/home/laba/butler-sso-20261102/reports/native-google-03-proof.json` and the
retained `native-google-03` observer directory. The safe confirmation screenshot
is an operator artifact, not a replacement for the durable RP/delivery checks.

The follow-up fix adds a read-only current-source draft preflight before Broker
preparation, bounded correlated non-dispatch feedback, and a verified projection
with the exact check URL and explicit Chrome/server-session separation. Pinned
terminal Task-publication absence review preserves the failed Input, consumed
UNKNOWN grant/dispatch and spending. New native acceptance remains required.
