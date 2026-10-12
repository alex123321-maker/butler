# Bounded owner state and controls, issue32

This acceptance covers the current owner product contract, not arbitrary task
quality. No model inference, human Telegram messages, real external writes or
main deployment mutations were performed. Web presentation uses labelled seeded
HTTP observations; the separate actual Chromium/Go/PG fixture exercises real
owner mutation, persistence, replay and branch boundaries.

| Criterion | Owning implementation/evidence | Bounded browser observation |
| --- | --- | --- |
| 1 Task differs from answer/Run | `ownerEvidence`, `TestOwnerTaskEvidenceHTTPKeepsResultsEffectsDeliveryAndReplayDistinctIntegration`; saved answer leaves task active, obsolete verification stays historical | `durable task keeps completed attempts separate from verified work`; actual API fixture retains answer/delivery metadata without completing task |
| 2 read/effect/delivery uncertainty | authenticated read errors remain unavailable; all notification chunks aggregate conservatively; same HTTP integration and safe journal projection | `unavailable evidence and delivery do not become successful states`, five result-state cases; real API fixture shows seeded confirmed/unknown effects separately from delivery_unknown |
| 3 initiative basis and deferral | current assignment/goal/mandate/source/observation/version projection; current initiative reason and estimates distinct from consumption | `initiative controls preserve quiet decisions, source revisions and current owner proof`, `owner observations distinguish child results, effects and incomplete shared usage` |
| 4 concrete approval without secrets | `OwnerFactsForRecord`; `TestOwnerApprovalHTTPProjectionOmitsPrivateStoredMaterial` checks actual list/get bodies, not just DOM; no raw argument/payload/details JSON or free summary/resolution | `approval shows exact server consequences and authority without private details` |
| 5 change/pause/resume/branch Stop/revoke | real task controller; exact-reference mandate revocation replays before CAS, immediately denies admission and propagates branch cancellation; two `TestOwnerMandateReference...Integration` cases | actual Chromium API fixture pauses/reloads/resumes/amends, stops only a child; `owner goals distinguish assumptions and revoke current mandate authority` |
| 6 children stay within parent work | owned direct children/results, independent parent revision and inline child Stop | actual API fixture and `branch stop uses its own observed revision and keeps the parent task current`; no separate dialog is required |
| 7 saved typed evidence | bounded current/obsolete results, safe source/artifact/effect/result refs, omission counts; raw source bodies/private URLs/paths excluded from snapshot | `owner sees pinned sources, saved artifacts, stale verification and separate delivery`, exact artifact/source selection tests |
| 8 install/readiness/queue/resources | existing reviewed-extension lifecycle, installed/setup/health/task permission remain independent; actual PG background job projection labels obsolete/deadline and reserves versus usage | `reviewed extension lifecycle keeps setup, health and task permission distinct`; snapshot queue and incomplete shared usage case |
| 9 concise journal and versions | finite current/stored authority observations, action contract hashes and pinned source revisions; no hidden reasoning/raw verifier/continuation | owner evidence case, approval wire assertion; Telegram short bounded observation text |
| 10 Web and text controls/dedup | actual HTTP lost-ACK/restarted handler receipt replay; real mandate PG replay; all Telegram controls use trusted numeric sender/chat/update identities | actual Chromium→HTTP→PG lost-ACK retry sends exact saved body/delivery and one durable event; `TestTelegramOwnerStateAndMandateCommandsUseTrustedActorAndFakeHTTP` uses fake local HTTP only |

The required restart/waiting scenario uses a durable owner waiting task and
reloads a committed paused task from PostgreSQL; the existing input/lease restart
tests own worker recovery. Seeded send-confirmed/notification-unknown and
initiative-budget-deferred observations verify display semantics without claiming
this run sent an external message or consumed an initiative budget. Child Stop
is an actual owner mutation. Extension installed-but-unconfigured remains an
independent presentation fixture backed by the existing extension lifecycle API.

## Exact lab evidence

Snapshot `20261012-151335-824`, isolated agent root
`/srv/storage/butler/agents/owner-state-ui`, includes root memory proof/selection
fixes and the workspace discard controller dependency.

- `reports/owner-state-20261012T081351Z-643009/tests.jsonl`: 75 passing names,
  67 top-level tests, zero failures/skips; affected race/vet and app compilation.
  SHA-256 `bceeefa7ac77a111c9b8f4799ab42fc177d0dd7b2fd3af2753ea78e020bfc8c2`.
- `reports/owner-browser-api-20261012T081351Z-643008/api.jsonl`: actual Go HTTP/PG
  fixture passed; independently checks root version4, cancelled child, lost-ACK
  task version2 and exactly one durable task control event.
  SHA-256 `a0a12066e0cc457b074e9cf3ea3b243c6fc670f99f5c9a6162f96cb30355e7f2`.
- Its `browser.log`: real Chromium owner API flow passed.
  SHA-256 `50fb8db660306a7ea3ec6ded09549ce3a40a9aff43d5db4bdbf643a1f0c18b16`.
- `reports/owner-state-web-20261012T081440Z.log`: TypeScript, lint, production
  build and 37 Chromium scenarios passed, zero skipped selected cases.
  SHA-256 `b67542401c8e311892d3ca89b95dfef2921f87b464c643c5c850a0798de84b11`.

Earlier failed evidence remains retained. The first PG fixture omitted required
owner verifier metadata and was refused correctly; its fixture was corrected.
The first actual API Chromium test reached the expected stored artifact twice
and failed an ambiguous locator; the assertion now identifies one exact link.
The broad initial Web run had 35 passing cases and one unrelated stale all-page
visual baseline: merged Skills/Converters sidebar entries predated this task,
while its old overview image had neither entry. Safe expected/actual images were
inspected; no overview/sidebar implementation was changed. The focused functional
runner explicitly excludes that unrelated screenshot test, preserving its failed
log rather than marking it passed or updating all screenshots blindly.

## Workspace owner adapter

The optional task workspace view discovers safe unknown-Create copy IDs from the
owning recovery controller. HTTP rejects caller paths/owners/leases/fences,
duplicate or Unicode-aliased keys, null/missing CAS, trailing JSON and unstable
delivery. Tests `TestOwnerWorkspaceRecoveryHTTPRejectsCallerAuthorityAndPreservesUnknown`
and `TestOwnerWorkspaceRecoveryDiscoveryRequiresOwnerAndUsesBoundedTaskProjection`
check the adapter boundary. Browser case `workspace discard discovers the copy,
keeps exact lost-ACK intent and original effect unknown` checks current proof and
same delivery/body retry. These adapter tests do not claim physical erasure;
the separate real executor/quiescence/receipt tests documented in
`workspace-owner-discard-acceptance.md` own that evidence. The original effect
remains unknown even after an independently verified discard receipt.

Issue closure, shared deployment activation and final integrated checks remain
with the root owner; this component does not close issues or change runtime flags.
