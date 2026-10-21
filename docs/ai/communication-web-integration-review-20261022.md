# Web review and communication integration — 2026-10-22

Reviewed base: PR131, `0aaba1b90c27db588e00db305aa70c06b2087e4f`, based on
main `d307a6c6a38e376bb56665cddb2c293b09538803`. Its eight GitHub checks passed.
The owner's later instruction authorizes Web integration. Production activation
and deployment remain separate; this record does not claim either or a merge.

## Review findings and fixes

- P2: run_error's short message bypassed the credential masking used by expanded
  event data. The summary now uses the same masker; the bearer regression test
  checks the whole emitted row.
- Integration blocker: HTTP metrics instrumentation discarded http.Flusher, so
  real run/publication SSE could return an error despite intercepted UI tests.
  Instrumentation now preserves the underlying streaming capability. A real
  HTTP server test reads a frame before the handler completes.
- Native shutdown exposed a second stream-lifecycle bug: HTTP request contexts
  outlived App.Run, so an open SSE could exceed the ten-second shutdown grace.
  HTTP BaseContext now follows the owning Run context. A subsequent actual App
  restart with an open authenticated Web stream passed (zero model calls).
- Cross-channel outage review found that a polling error could terminate the
  explicit Telegram adapter and consequently the whole App/Web. Read-only
  polling now uses cancellable bounded backoff while retaining its durable
  cursor. It never retries an external send or starts another model turn.
- The replacement UI itself did not implement #129: it read flattened transcript
  messages and had no canonical report/media/reply feed. The integration adds a
  separately gated Web projection without enabling ordinary startup.
- Native attachment review found forced `protect_content` prevented Telegram
  saving. Authorized report exports now omit that flag; source/export guards
  still run before delivery. Fresh native client qualification is in progress.
- Actual native missing-response tests found two defects hidden by the scripted
  provider: the reminder replaced immutable system instructions, and the native
  transport discarded its context before Core could request correction. The
  reminder now uses user-role continuation under existing authority. Successful
  text finals retain a private context for at most one minute; cancellation or
  expiry removes it without inference. Native quiet and corrected branches now
  pass separately. Old expiry callbacks cannot discard a newer turn.
- CI exposed a voice fixture race: setInputFiles fired while the availability
  request still disabled input, so chooseFile correctly ignored it. The test now
  waits for the enabled input, preserving production microphone/privacy guards.

## Implemented integration

One canonical publication fans out to its pinned Web and Telegram destinations.
Web serves the full saved report with all typed blocks and required media;
Telegram retains its immutable native parts. Availability, channel ACK, read and
Task completion remain separate. A current owner/binding/source/artifact check
protects feed, detail, stream reconnect and authenticated media download.
Web replies retain the original publication/session/task, and their owning Input
commit writes immutable private ingress proof. A new Web conversation chooses no
Task from the selected panel. Missing profile readiness blocks new sends; a
legacy startup's explicit404 preserves the old ingress.

SSE sends only cursor invalidation. Loaded pages come from PostgreSQL REST and
are replaced after reconnect/refresh, including redaction and independent
delivery changes. Media uses temporary object URLs after digest/size validation;
owner/source changes remove the corresponding view. No raw HTML/JS/remote media
URLs or executable report blocks are rendered. Cursor reads create no transcript
copies or delivery ACKs. Prior pending history is not upgraded to available.

## Evidence in progress

- Twelve focused Chromium cases passed, including the six new communication
  cases and the journal suite. Actual browser download returned exact fixture
  bytes. XSS-like text remained text; a 20-column Unicode table fit a320px view.
- Fresh typecheck passed after fixing a test-local variable that shadowed the
  browser document. Both linters passed. The first complete run had76 passes,
  2 real-environment skips and3 fixture failures caused by a missing explicit
  legacy-profile response. Its helper now returns404; all5 affected Doctor/voice
  cases passed. The final complete Chromium run passed79 with2 real-environment
  skips. CI's Playwright installer rejected ubuntu-latest26.04; it now uses the
  pinned supported Ubuntu24.04. Final CI is still required.
- New PostgreSQL tests passed for canonical full report/file reads, cursor,
  actual authenticated SSE and owner/binding/source/media revocation. The first
  wider run exposed old pending-Web fixture assumptions; availability is now
  explicitly gated by the composed Web capability. Fresh Linux/race rerun:
  365 test/subtest passes,0 failures,0 skips; vet passed.
- Ten composed Core/Broker Web+Telegram scenarios passed in the first run,
  including final/correction/quiet, receipt recovery and exact Web reply retry
  with immutable private ingress proof. These are synthetic model scenarios.
- Actual App.Run + operator Broker + native model + both clients admitted a real
  Web request, delivered early progress before subsequent tools/final, then a
  photo and native full report. A real Telegram reply and explicit Web reply
  continued `web-explicit:application-web`; eight native turns and five
  publications were observed. Owner approvals used the actual Web mechanism.
  All three runs completed. The lab then failed its manual-stop deadline and
  exposed the HTTP shutdown bug above, so this is not a clean lab pass. Usage
  and scoped client observations are retained; the test schema was cleaned before
  manual export. Deferred private durable evidence capture is now mandatory.
  Repeat qualification includes a same-profile restart, without resetting budgets.
- Trusted DrainOnly preserves readers, retained inputs and outbox while refusing
  new ingress atomically. A focused real PostgreSQL/race test passed for exact
  lost-ACK replay, rejection rollback, epoch-0 fencing and epoch-1 queue draining.
  Eight focused Web cases passed, including readable history with disabled new
  sends. The lifecycle evidence capture test also passed against PostgreSQL.

No issue is closed by these partial results. #129 requires the same live fixture
in Web and the real Telegram client, including document open/download. #130 also
retains actual application/early delivery, native quiet/missing behavior, full
usage accounting, compatibility/rollback/drain and the unweakened linked AC gates.

## Actual App qualification, retained window v6

The isolated App and operator Broker ran from18:43:21 to18:47:56 UTC on
2026-10-21 (2026-10-22 in Asia/Tomsk). `TestNativeCommunicationApplicationLab`
passed in275.22s. Three real owner inputs completed in the same canonical session
`web-explicit:application-web`: Web report request, native Telegram reply to that
report, and explicit Web reply to that same report. Six native turns produced
three canonical publications, three Web available targets and three independently
sent Telegram targets. Approvals used the real owner Web flow.

Report `publication:186c7739d90296036dd65b122acc9e1031a48ac2aa90c95339576155f43c95b8`
appeared in Telegram Web K as message84400 and in Butler Mind as the identical
canonical publication. It contains a Unicode table, a PNG image and retained
PNG/TXT document blocks. Telegram reply produced
`publication:cc181323b82a00db95c993db77963fc34d16722b5e1fbfdd0910f109979a9eac`;
Web reply produced
`publication:2042be2687a4760976f5879ef942ad4a091f384069042606d644777c03ac070b`
(Telegram message84403). All three Input receipts and runs completed; no semantic
external-task verification is claimed for these synthetic fixtures.

A same-profile restart retained all three IDs, source checks, binding/cursor and
six recorded attempts. DrainOnly then disabled new Web messages/replies while
the full report and its media remained readable. Final counts stayed unchanged.
An authenticated Web stream was open during shutdown/restart; both stops were
clean. Proxy trace:5 connection requests,222 HTTP connection uses,0 direct bypasses.
Deferred private SQL capture ran before schema cleanup. Ordinary startup and all
production services/configuration remain unchanged.

Telegram filenames showed downloaded state, but that CSS state and Bot API ACK
do not prove client file opening/saving. The owner was asked to verify the new
unprotected PNG in report v6; this criterion remains pending until confirmed.
Supported observed client: Telegram Web K `/k/` on the above date. Its public DOM
loads `index-BcovhLpe.js` and `app-BMaWPa4l.js`; these observed bundle fingerprints
identify the checked build without inventing an unavailable semantic version.
Planner bounds are not certified Bot API maxima or evidence for untested clients.

| Retained artifact | SHA256 |
| --- | --- |
| App source application-v5 | `02e252d93a3fff6d3dc2860982a65db30f5077b09f7e262c829c3ef2bc9291c3` |
| App orchestrator binary | `a6261979b0834b78ceac84af49d9277c36ec015cda05bfb1b1465e9aaf11164f` |
| Actual Broker command | `297d286ac755f17a6878b0077e1a9e160a40dcdbc210fe0a6f84a872911c29b9` |
| v6 App log | `73e1b7294b1cc5eb749bb4f63b5060611670ffc53675f806a7672136cc62bf91` |
| v6 private durable evidence | `ec29d2b94eed63f9f66e60898cd97648f2f8d98797eb77b0966fbb7ea9151793` |
| Native correction source application-v8 | `561b4fe748a30d4ede08f3f1d151b0336bd486771bf8479416c47a3b01eeefdc` |
| Native correction orchestrator binary | `170ffec57d8c7cf620716224026058049c7c3037848ecfe72c266ae67374a61a` |
| Passed quiet/correction log | `516cd96093acfa3a78bcbf1683685bc00757fdfa45d0b6223bf48687f9efe945` |

Private root remains `/srv/storage/butler/agents/communication-130-20261021`.
The operator runner now changes permissions only on private files: a broad
chmod600 had removed directory traversal while preserving evidence; owned
evidence directories were restored to0700. No unrelated files/volumes were removed.

## Complete native denominator and remaining gates

Frozen earlier windows remain in [prior acceptance](communication-agent-acceptance-20261021.md).
Additional retained windows, including failures:

| Window | Durable attempts / observed / unknown | Input / visible output / provider total, observed only |
| --- | --- | --- |
| Earlier native windows incl. v20 |39 /27 /12|60872 /1966 /136288|
| Actual App v1 (deadline/shutdown failure)|8 /8 /0|20385 /822 /29492|
| Lifecycle restart only|0 /0 /0|0 /0 /0|
| Actual App v5 (preparation failure, queued successor)|3 /3 /0|7040 /583 /15717|
| Actual App v6 (clean pass)|6 /6 /0|13648 /470 /19404|
| Quiet/correction v1 (system-role rejection)|3 /2 /1|3901 /25 /4687|
| Quiet/correction v2 (discarded native context)|3 /2 /1|3901 /26 /4884|
| Quiet/correction v3 (clean pass)|4 /4 /0|8475 /81 /10745|
| **All retained windows**|**66 /52 /14**|**118222 /3973 /221217**|

Two unknown ledger rows are known pre-inference continuation rejections; they
remain unobserved rather than historical backfilled zeros. Provider totals include
thinking and are independent of visible output. Subscription/no-overage is the
configured price basis, not an invoice or proof that inference is free. Failed
PostgreSQL preparations on the full root disk made no model request; test data
were moved to fresh owned `/srv/storage` directories, with old evidence retained.

Passed native quiet: one turn, zero publications. Passed missing response: three
turns, one durable correction, one explicit result and native message ACK29.
No repeat action, implicit final send, alternate model/account or budget reset
was used to qualify either branch.

Focused final Web checks:10 communication cases plus3 retained voice cases pass;
typecheck and both linters pass. Native transport full race suite and vet pass;
four focused actual PostgreSQL/Core/Broker correction/quiet scenarios pass. Final
GitHub CI on the final revision remains a separate gate.

The bounded Unicode/table/multipart/media corpus and its local oversized/nesting
rejections combine [prior Telegram evidence](communication-telegram-rich-progress-20261021.md),
targeted tests and the actual server corpus below. Independent native file-open
still requires its separate owner confirmation.

## Native format/server corpus (zero inference)

`TestNativeCommunicationTelegramFormatCorpus` pins only the dedicated test bot
and owner, requires the private proxy file and makes one attempt per fixed
synthetic request. It is an operator transport test, not a substitute for the
actual App/Broker qualification above. No Stars/paid broadcast or provider request.
The [official Bot API](https://core.telegram.org/bots/api#sendmessage) documents
the text bound; rich article maxima are not published in the inspected schema.

| Fixture | Observed result |
| --- | --- |
|4096 Cyrillic characters,8192 bytes|HTTP200; returned text exactly equal|
|4097 Cyrillic characters|HTTP400/error_code400; no send retry|
|2048 emoji,4096 UTF-16 units,8192 bytes|HTTP200; exact text preserved|
|50 rich blocks and Unicode20-column ×3-row table|native ACK; Web K renders20 columns/3 rows|
|One complete32768-byte manifest, including indivisible code block|native ACK; no truncation|
|Indivisible block whose standalone manifest is32769 bytes|local rejection before HTTP|
|Nested canonical report block|typed schema rejection before HTTP|
|PNG/photo/caption + PNG/TXT documents|actual App v6 report/media evidence above; file-open remains pending|
|55 retained blocks across2 native parts after process death|prior native process-restart evidence; no replay|

Final corpus passed7 cases in1.59s; native message ACK38/39 and rich ACK40/41.
Earlier corpus v1 passed the30KiB fixture. Corpus v2's negative fixture failed
because it overflowed the combined title+body part but still fit as a standalone
block: splitting was correct. The final fixture measures the indivisible block
alone. Both negative logs are retained; no product limit was weakened. All three
corpus windows made zero model requests, so native usage totals above are unchanged.
Only the observed Web K build is qualified for rendering, and only the implemented
flat report profile is claimed.32KiB/50-block/20-column limits remain conservative
Butler bounds; acceptance of those fixtures is not discovery of Telegram maxima.

Final source `source.application-v11.tgz` SHA256
`87be66f31e3e144a1b7576f74742fa044ee853934845a39401633c0e28ed4e60`;
native format binary `0d2fd752a29a0c2d618ca191b3f879dc02f2e63cbbad435127f5e17e04cf7107`;
log `798e354271915cba3aeb7d27fdbd5580adbc819077cc4ad86a1820cbf1fe2ee7`.

For [#76](https://github.com/alex123321-maker/butler/issues/76), this is a bounded
failure→hypothesis→fix→native rerun record with retained negative observations.
It is not a comparative behavioural improvement study. [#105](https://github.com/alex123321-maker/butler/issues/105)
retains widget/multi-run/verified-task semantics and semantic response grading;
[#106](https://github.com/alex123321-maker/butler/issues/106) retains full Doctor,
operator migration and rollout; [#107](https://github.com/alex123321-maker/butler/issues/107)
retains the complete AC01–AC22 release matrix, pizza/widget flows and production
qualification. None is closed or weakened by this integration.
