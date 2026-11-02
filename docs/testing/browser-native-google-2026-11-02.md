# External Google native test, 2026-11-02

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
