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
| 9. Repeated project error produces a checked prepared correction | Three real Broker failed checks produce one initiative and ordered input; ordinary Core obtains one actual stored patch, checks it and retains `applied=false`; owner checks the stored proposal and actual check before task verification | Live `gemini3.8flashhigh` four-turn run, separately sealed and bounded |

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
