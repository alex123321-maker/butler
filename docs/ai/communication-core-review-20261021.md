# Review of PR127 — 2026-10-21

Historical review at `a9ccc38f`. The remaining backend criteria below are addressed
by the subsequent [full backend acceptance](communication-backend-full-acceptance-20261021.md);
this report preserves the earlier findings and evidence without rewriting history.

Reviewed the actual backend diff at `8e3b667bf38dac27260bcab5f8dd9a37baf4b865`,
the architecture contracts and current #99/#100/#103 criteria. All seven GitHub
checks for that original revision succeeded: Go, Android build, emulators31/33/
35/36 and tool discovery. CI annotations concern existing Actions/runner
deprecations; they do not establish communication acceptance.

This review corrects dormant backend behavior only. It neither changes Web or
the existing Telegram adapter nor installs Bootstrap hooks, an admission profile,
feature flag or a channel sender. No merge, deployment or native model run is
part of the review.

## Findings and fixes

| Finding | Correction and verification |
| --- | --- |
| A dispatched publication did not recheck owner goal/mandate authority | The owning store checks current authority under Task locks; an actual Broker claim followed by pausing the goal cannot publish, even with unchanged Task revision |
| Admission checked token names but not the actual fence/task binding; a stale caller could borrow another live Input claim | Admission verifies live DB ownership, Task binding and private claim. Publication, correction and explicit completion match the caller's exact private claim; takeover-before-lease tests deny the old caller |
| Input and dispatch/approval expiry could pass during export-policy waits | Recheck owning input/source/authority after export; recheck actual private dispatch/grant/approval immediately before commit. Expiry-during-wait tests assert that the wait was reached and that every staged publication/outbox row rolls back |
| Private service bearer token could be sent to arbitrary plain-HTTP hosts | Only literal loopback or the exact owning Compose HTTP destination is allowed; other hosts require HTTPS. URL boundary tests cover the rejected and allowed destinations |
| Quiet/verified_task could fail completion after an answered optional question, or report ready while a question was still open | Waiting takes precedence; an answered quiet obligation becomes satisfied before completion. Actual Core/PG fresh-context resume covers quiet and verified_task without a new result publication |
| Failure to admit/finish the single correction abandoned an open obligation | The same fixed failure-closure path runs without another inference; cancellation/stale ownership retains denial. Tests cover admission and broken-stream failures |
| First100 healthy no-expiry questions could starve all later expired questions | Select invalid/due questions before LIMIT100 and revalidate under owning locks. A batch with100 older healthy waits still expires the later due question |
| A queued widget could still send its old open question after expiry/resolution/cancellation | Pre-send authorization checks current interaction state and DB expiry, including taskless questions; old open-widget representations fail before send |
| Stop/amend after a response left the Input's obligation open | Cancel all unfulfilled obligations on the controlled tasks, preserving the already accepted response DATA; real Task Controller tests cover both controls |
| Nonempty explicit final could bypass terminal proof at Completion | Explicit completion requires typed terminal evidence regardless of text length; rejected persistence leaves no partial final result |
| A failed wait-state read was treated as absence and could overwrite the question with a generic tool result | Distinguish absence from a failed read; typed bounded verification preserves the old wait/early answer and does not fail the waiting Run, continue the provider or repeat a tool. Core tests cover ACK/lost ACK/read failure/missing marker; actual Input worker tests cover waiting, early-ready and unknown-dispatch checkpoints |

Reproductions before fixes are retained locally in
`.cache/communication-core/review-red.jsonl` and `review-red2.jsonl`. New tests
exercise real PostgreSQL and owning services where relevant; provider/channel
boundaries use controlled fixtures and do not claim a live Telegram/Web test.

## Verification scope

The required final-source selection is `scripts/dev/communication-core-tests.sh`:
communication contracts, exact action/budget/Broker boundaries, owning control,
Core/waits, compaction/extraction with `-race -count=1`, and no skipped required
cases. `--regression` additionally runs affected existing packages, compiles and
uses the actual private Broker harness for shared control regression, vets the
affected packages and builds both services. Tests use disposable local Linux
Go1.27.2 and PostgreSQL17/pgvector only, with repo-local caches. Reports are
`targeted.jsonl`, `regression.jsonl`, `control-regression.jsonl` under the same
cache directory. Native/backup/browser tests outside this slice are not claimed
as passed; skips are recorded explicitly.

The final-source run completed successfully at 07:25 UTC on 2026-10-21:

- 42 distinct top-level communication tests passed with the race detector;
  zero failures or skips (subtests are not counted as separate tests).
- 12 shared private-control tests passed with the race detector, including
  the actual Broker subprocess harness; zero failures or skips.
- 383 affected regression tests passed; eight tests were skipped, not passed.
- Scoped `go vet`, both service builds and `git diff --check` succeeded.
- The diff against `origin/main` leaves Web, existing channel adapters,
  Bootstrap composition and Go dependency files unchanged.

Regression skips: `TestLegacyTaskMigrationIntegration` (exclusive migration DB),
`TestExecuteIntegrationPersistsRunAndTranscript` and
`TestPipelineWorkerIntegration` (Redis), `TestBrokerBrowserSequenceE2E`,
`TestResourceBrokerActualChromiumPageCASIntegration` and
`TestWebToolsBrokerSixDefectsIntegration` (actual browser services),
`TestDelegationBrokerHarness` (subprocess-only helper, separately exercised by
the shared control run), and
`TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration` (backup DB).
Names excluded by the script's Native/Backup/ServerBrowser selector are also
outside this claim. None of these establish communication acceptance.

The original revision's seven green CI checks above are historical evidence.
The review revision must independently pass the checks shown on
[PR127](https://github.com/alex123321-maker/butler/pull/127/checks).

## Issue closure assessment

| Issue | Decision and unmet original criteria |
| --- | --- |
| #99 | Keep open: ordinary cross-channel reply target/ambiguity resolver and the complete native Rich Message fixture contract have not been implemented/accepted. Schema and internal binding checks do not prove those criteria |
| #100 | Keep open: the original mandatory matrix includes failures at each transaction boundary, complete Broker/Core message continuation, concurrent delivery/known-absence retries and ACK-persistence failures. Current component/PG evidence is useful but does not establish the entire matrix |
| #103 | Keep open: the complete crash-at-each-handoff/lost-ACK/fresh-ownership/authority matrix has not been run as required by the issue. Working controlled resume, clock simulation and first-response tests do not establish every interruption/revocation boundary |

These are remaining acceptance requirements, not new requirements introduced to
block closure. The backend slice remains independently reviewable while channel
refactoring is deferred. No checklist is marked complete merely because CI is
green. Future channel composition still needs current projections for resolved
questions, owner APIs, real export/artifact policy and channel ACK mapping.
