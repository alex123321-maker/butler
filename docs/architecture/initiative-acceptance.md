# Initiative acceptance evidence

Issue #15 has nine criteria. Durable subsystem tests and the ordinary-task
real-process fixture cover the component gates below. Live High planning is a
separate gate: a scripted provider is not evidence of generative quality.

| Issue criterion | Owning implementation and evidence | Remaining gate |
| --- | --- | --- |
| 1. Durable schedules, timezone and DST | PostgreSQL source/schedule/tick state; `TestCalendarHandlesMissingAndRepeatedLocalMinutes`, `TestChangedScheduleCannotRecordOldTickIntegration` | None at component level |
| 2. Downtime does not replay a backlog | Phase-preserving latest-tick coalescing, skip policy and leased ticks; `TestDowntimeCoalescesOnceAndKeepsIntervalPhase`, `TestScheduleDowntimeLeaseRecoveryAndAcknowledgementIntegration` | None at component level |
| 3. Deduplication checks current source and completed work | Semantic identity retains owner/resource revision; real Broker attempts count once per run; `TestDistinctEvidenceAndDeliveryDeduplicationIntegration`, `TestBrokerProjectPromotionRequiresLiveSourceReleaseIntegration`, `TestVerifiedProjectResultClearsUnconsumedFailuresIntegration`; ordinary fixture replays the third actual attempt | None at component level |
| 4. Own changes do not cause an initiative loop | Origin comes from Task Service, never response prose; both installed observers and ordinary fixture suppress the promoted task's own check | None at component level |
| 5. Ordinary tasks inherit mandates, budgets, verification and stops | Atomic Task Service/ordered Input admission, current owner-goal mandate, shared finite execution policy; ordinary Core runs four provider turns through actual Broker; missing/exhausted budget, authority/source changes and atomic owner tree Stop have separate actual PG regressions | None at component level |
| 6. Active hours, quiet checks and grouped delivery | Admission rechecks the PG clock after budget waits; empty checks/proposals have no notice; digest source/goal/mandate is checked at claim and send; production Telegram adapter obtains an actual synthetic HTTP ACK once | None at component level |
| 7. Safe pull audit explains the initiative | Owner view exposes bounded typed source/observation IDs, status, deterministic reasons and estimates; raw errors, model content and credentials are excluded | None at component level |
| 8. Model, external effects and verified task outcomes stay separate | Actual Broker dispatch/recipient confirmation is required; model final remains `unverified`; independent owner verification completes the task; digest ACK is separate; low-confidence observations remain proposal-only | None at component level |
| 9. Repeated project error produces a checked prepared correction | Three real Broker failed checks produce one initiative and ordered input; ordinary Core obtains one actual stored patch, checks it and retains `applied=false`; owner checks the stored proposal and actual check before task verification | Separately sealed actual High four-turn run passed; bounded evidence below |

`TestInitiativeProjectChecksRealBrokerE2E` uses a disposable PostgreSQL schema,
real Tool Broker and extension runtime subprocesses, a reviewed fixed-origin
project fixture and an authenticated owner goal/mandate. Three inexpensive
fixture check tasks issue exactly approved GETs. Their completed Broker
authorization, intent/effect journal and Transcript Store records supply the
observer input. A second GET in the first run and a replayed third observation
do not count as independent failures. No tool journal or effect is fabricated.
The initial observation drivers deliberately use no model.

The promoted task is executed by the ordinary durable Input worker and Core,
not by a replacement tool driver. The injected component provider calls inspect,
prepare-patch and check, then returns one strict artifact/check projection. The
Core and Broker use the same finite budget policy and current Catalog pins.
The owner inspects the recipient's actual `patches.json`: exactly one proposal
must contain the original revision and exact README correction. A saved Broker
GET proves checks passed and `applied=false`; the original remains unapplied.
Task Service rejects completion using the unverified model result. Only the
independent owner's verified criteria/evidence result completes the task and
initiative. The model final and its hash remain unchanged and unverified.

After verification, repeated queueing produces one count-only digest. Production
Telegram `Adapter.SendStored` sends through its actual HTTP client to a synthetic
Bot API endpoint for `telegram:chat:424242`. The endpoint requires the count-only
message and returns ACK101. The durable outbox records `sent` with that ACK; a
second worker pass sends nothing. No human message or operator Bot token is used.
Queueing after completion truthfully reports zero new starts and one verified
result; this does not claim receipt or reading by a human.

The fixture, strict projection and all initiative tests passed on 2026-10-12
with race detection and vet in isolated snapshot `20261012-144445-723`, report
`broad-20261012T074453Z-535083`: 57 passing tests, zero native inference,
zero failures and zero skips. Events SHA256:
`65db521039e4e328abc36efa39c996b109a0845a8976eb87702832169418c085`.
The earlier focused proof is report `narrow-20261012T073805Z-510330`, events
SHA256 `cd306984c1a7a84a55718cdccead00d8e3bac974b06661c26f9831bde5801903`.

`TestInitiativeHighOwningBindingsZeroInferenceE2E` additionally selects the
explicit High context while retaining the clearly scripted transport. At every
actual ordinary Start/Submit entry, the native route's current task/initiative/
source/goal/mandate SQL binding must accept the promoted run. Neither the native
provider nor attempt reservation is called: the native counter stays at41 with
zero journal rows. Together with the unowned-route rejection, projection and
ordinary fixture tests it passed four focused PG/race tests and vet in snapshot
`20261012-145057-601`, report `narrow-20261012T075103Z-559800`, zero skips.
Events SHA256: `138d5278976b7fb5f4786393266969b6e204560bcf9e93ccddd0093152710386`.
This checks route composition, not live High acceptance.

The actual `gemini-3.8-flash-high` continuation passed all four promoted-task
turns42–45 on release `250cf05be67b3ba19bcdcc6a7afabf13d5733e79` in29.32s.
Safe report SHA256 `0ded1f418c3ecf9fad177fe5141350e8c5ca8002452fb462e3114219c4ab4ca9`
confirms the ordinary Core/artifact/channel checks, independent owner completion,
unverified model final and one synthetic digest ACK101. The cheap observation
drivers remain model-free and the preflight transports remain explicitly
scripted. Independent review accepted all nine criteria within this declared
synthetic project; no production deployment or automatic general verifier is
claimed. Shared owner-day exhaustion has separate actual Ledger/PG evidence in
`TestBudgetRootPreflightDoesNotChargeOrResetAdmissionIntegration`.
Three tool-turn usage pairs remain NULL, while the final pair is5640/75. They
remain charged and unknown; full accounting and source/window pins are in the
[bounded High acceptance](../ai/initiative-high-acceptance-20261012.md).

Run `scripts/dev/initiative-lab.sh <isolated-agent-root> broad` from a read-only
source snapshot populated by `scripts/dev/lab.ps1 -Action Sync`. It builds the
three actual-process binaries, runs the app fixture and all initiative tests
against disposable PostgreSQL, checks race/vet and rejects skips. No native
authentication is mounted. The live caller must use the separate immutable
history and four-attempt High window documented in
`docs/ai/initiative-high-acceptance-20261012.md`; a component pass does not seal
that future native window as successful.

Historical proof `20261011-061804-374` established the same actual Broker,
recipient, source and independent verification boundaries with an explicit
prepared-correction driver. It did not establish ordinary generative execution
or actual channel delivery. The current fixture retains those boundaries and
adds ordinary Core/Input execution and the actual adapter/HTTP ACK.
