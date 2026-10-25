# PR #155 review and working-profile acceptance

## Review fixes

1. The native semantic gate used substring matching and could accept the expected
   answer followed by unrelated historical facts. It now compares the complete
   finite answer, allowing only case/punctuation and the documented fruit order.
   Negative cases include appended ribbon/anime facts and previous selections.
2. The Core/separate-Broker request-scope integration was absent from mandatory
   Communication CI selection. `TestCurrentRequest*` is now included; native
   inference remains separately gated and is not part of credential-free CI.
3. Real Web acceptance exposed a history-pagination race: a click on “Load more”
   during a background read set the refresh flag but discarded the larger window.
   On the retained long conversation, the current question stayed outside the
   loaded history. Preserve the explicit expansion before deferring the read.
   The new Chromium regression holds a background response, clicks expansion,
   releases the response, and requires the 51st publication exactly once. It
   fails on the old ordering (current question absent) and passes on the fix.

The review retained PR #150's recent-history selection, authority checks,
recovery, budget accounting and stopped-attempt policy. #150 merged as
`bd658c7aa9b6d84f8627fa552c4e317fc7bdb3e6`; #155 was retargeted to main and that
merge was incorporated without production conflicts or duplicated changes.

## Actual channel procedure

The operator used the existing signed-in Telegram Web account and the lab bot,
plus the normal Butler Web owner form. The configured owner token was entered
directly without printing it; the temporary local handoff file was removed.
Messages and responses were submitted through the visible interfaces, not
synthetic ingress, fake Bot API updates or direct database writes.

LAN HTTP was unreachable from the desktop, so SSH forwarding carried Web/API
traffic. An isolated Web container used the current frontend build and the
shared lab Core, PostgreSQL, provider, owner bindings and existing conversations.
Only its public API address was adapted for the tunnel. The shared Web service
and other agents' containers were not replaced for this qualification.

Core was built from the production sources present at `47f9429c`, with executable
SHA256 `10aecd518d3ca819c737ccfb154138cd0817a8174274bfde31ca08b144f8316e`.
The shared Core was updated after confirming no queued/running/waiting inputs.
The first startup failed because the uploaded binary lacked executable mode;
the deployment helper automatically restored the prior Compose configuration
and Core. After correcting the mode, startup, executable hash and health passed.
The previously missing main migration `100_communication_owner_limits.up.sql`
was applied; migration102 remained installed. Broker and other runtime binaries
were unchanged. No history, uncertain effects or budget counters were cleared.

The Web build contains the pagination fix at `cdfbab6e`; its uploaded archive
SHA256 is `87e50a58ffe5647c5bc8fb359ea7c6a7775c06dcb66ab0e06c07a0873f480bb4`.
An operator-only Python stdlib helper handled Compose JSON snapshots, bounded SQL
reads and artifact hashes without interpolating credentials into shell commands.
It is not a product service or runtime dependency.

| Scenario | Actual input/answer channel | Required outcome |
| --- | --- | --- |
| Ask which fruits to bring, select apples and plums | Telegram message and multi-choice callbacks/Submit | One new question; exactly `Яблоки, сливы` |
| Change topic to the picnic tablecloth | Web message and text widget | Exactly `Белую льняную скатерть с зелёной каймой.` |
| Repeat the identical original fruit request, choose pears | Telegram message and fresh callbacks/Submit | A different canonical interaction; exactly `Груши` |
| Explicitly recall the previous tablecloth answer | Web message | Same cloth, no new interaction |

The tablecloth question was the only active input when Core was restarted at
04:55:14 UTC. The owner then answered its existing Web widget after restart.
The saved run/input identity was retained, phase changed from `owner_request` to
`answered_question`, and the exact original interaction ID was used. The answer
was published once and delivered to Telegram as well as made available in Web.
Fruit and cloth requests overlapped across channels; their canonical answers
and published results remained separate. The old ribbon/anime history remained
in the conversation and did not appear in any of the four new results.

Evidence is in `docs/evidence/issue147-current-request-20261025/review/`:
`channel-evidence.json`, deployment/restart records and screenshots. Web
`result_ready` means available for owner reads; Telegram `sent` includes actual
Bot API message acknowledgements. Browser observations independently establish
that the owner saw the results. Publication, run completion and delivery are
not treated as the same event.

The first four requests produced all four expected visible results. Three Inputs
completed; the repeated-fruit Input instead failed with `provider_protocol_error`
on the model call **after** the confirmed `Груши` publication.
Its first four calls have measured usage; the fifth retains NULL input/output
usage. No extra action was proposed by that call. The stored safe diagnostic
does not establish the provider's exact protocol failure, so no speculative
parser relaxation or automatic retry was introduced. This is a partial runtime
failure, not a passing end-to-end run; its error, failed status, retained
publication and unknown usage remain in the evidence and lab database.

One independent new owner request was then submitted through Telegram with the
same wording. The preselected acceptance answer for that window was only
`Сливы`. It creates a new Input/run/question instead of replaying or changing
the failed Input, and uses the unchanged working-profile spending limits.
That independent Input completed with exactly `Сливы`, a fourth distinct
canonical question and all five model calls measured. Across the five live
requests: five correct published/delivered results, four completed Inputs and
one failed Input after delivery. The 23 foreground model calls include 22 with
measured input/output counters (122,640 / 1,558 tokens) and one unknown pair;
these sums are not presented as complete provider-total spending.

After acceptance and a zero-active-input check, the same tested Web artifact
was installed in the shared Web service with its existing LAN API address.
`web-deployment.json` records the archive hash and HTTP200 check. The change only
replaced Web's output mount; the Core candidate and all other services retained
their versions. Private Compose rollback snapshots remain beside both releases.

## Validation and limits

The request-context unit/wire/vet script and the stricter semantic-gate unit test
passed. Web typecheck, lint, production build and all 16 communication-feed
Chromium cases passed. The new pagination case was additionally run against the
old ordering (failed for the expected missing publication) and restored code
(passed). Earlier paired native/Core/Broker and PostgreSQL evidence remains in
`issue147-current-request-2026-10-25.md`, under its original revisions.

These four real scenarios establish compatibility and correct request/answer
boundaries in the retained working profile. The earlier native baseline also
passed; neither experiment proves universal elimination of an intermittent
model error. Long-history loading remains slow because full access proofs are
checked for each publication; this PR fixes losing the user's expansion request,
not that separate latency problem. Issue #147 remains open for observation.
