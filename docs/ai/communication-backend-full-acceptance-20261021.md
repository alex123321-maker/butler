# Communication backend acceptance — #99, #100, backend #103

Date: 2026-10-21. Base: `origin/main ab28dd88`; follow-up to PR127 review
`a9ccc38f`. This document maps the original issue criteria to backend evidence.
It does not replace them with weaker criteria. The feature remains dormant:
Web, the existing Telegram adapter, Bootstrap registration and production
configuration are unchanged. There is no new product enable flag, deployment or
live channel/model call. Photo/report remain contract-only and cannot be admitted.

## What was completed

- Current verified binding/version + stored Telegram sent ACK or Web publication
  reference resolves an ordinary reply to its canonical session/task. Ambiguity
  requires an explicit target; sessions are never merged. Existing Input
  acceptance provides once-only stable delivery identity and payload conflict.
- A separately constructed owner-authenticated HTTP handler manages Web bindings,
  accepts ordinary replies and typed responses. It is not registered by Server or
  Bootstrap. Body/header owner IDs and arbitrary recipients do not authenticate.
- Version1 fixtures freeze the four formats, three blocking input types and a
  separately authored native Rich Message representation including photo/document
  blocks. These are shape fixtures for C01, not a C06 renderer or live send.
- The trusted optional admission hook runs after Task/source preparation, before
  catalog selection. Identity is derived from durable state; preparation recovery
  accepts only the same pinned profile. Startup installs no hook.
- Private approved widget publication now returns a saved wait through recovery
  without poisoning the run as a continuation failure.
- Denied exports use existing outbox `failed/policy_denied`, empty body and empty
  representation. Other permitted destinations continue. All-target denial fails
  publication. Respond rechecks current export policy under binding locks; send
  rechecks claim, binding, task/mandate and question deadlines after policy waits.

## Criterion mapping

Names below are actual Go tests. Communication-module tests live in
`apps/orchestrator/internal/communication`, unless another package is named.

| Original criterion | Evidence |
| --- | --- |
| #99.1 Model schemas contain no owner/chat/token/lease/grant authority; forged bindings denied | `internal/communicationcontrol.TestCommunicationStrictIntentAndBlockingQuestions`; `TestCommunicationOwnerHTTPProofAndStrictBodiesIntegration`; `TestCommunicationReplyProofAndBindingRevocationIntegration` |
| #99.2 One owner has two verified channels; new/rebound target gets no old backlog | `TestCommunicationNewOrReboundBindingNeverReplaysBacklogIntegration`; `TestCommunicationCrossChannelReplyCanonicalTargetIntegration` |
| #99.3 Owner decisions/native Rich Message frozen; no forms/nonblocking V1 | `internal/communicationcontrol.TestCommunicationV1FormatAndNativeRichMessageCorpus`, `TestCommunicationStrictIntentAndBlockingQuestions`, `TestCommunicationTypedAnswers`; `TestCommunicationMultiAndTextResponsesRemainDataIntegration` |
| #99.4 Publications/final have different identities; ordinary reply has proved target or explicit ambiguity | `TestCommunicationPublicationAtomicIdentityAndSeparateFinalIntegration`; `TestCommunicationCrossChannelReplyCanonicalTargetIntegration`; `TestCommunicationACKAmbiguityNeverChoosesLatestPublicationIntegration` |
| #100.1 Mid-run publication survives crash independently of final; accepted is persistence | `TestCommunicationEveryPublicationWriteFailureAndProcessCrashIntegration`, including commit then abrupt worker death; `app.TestCommunicationActualCoreBrokerTwoMessagesAndFrozenFinalIntegration` |
| #100.2 Exact replay returns original receipt; changed body conflicts; lost ACK reads receipt | `TestCommunicationPublicationAtomicIdentityAndSeparateFinalIntegration`; `app.TestCommunicationOwningDispatchAndReceiptRecoveryIntegration`; actual Core/Broker `lost_private_ack` case |
| #100.3 Any failed write rolls back; one failed target does not block others | Every-publication-write matrix; `TestCommunicationConcurrentDeliveryAndKnownAbsenceMatrixIntegration`; `TestCommunicationActualACKPersistenceFailureNeverResendsIntegration`; `TestCommunicationDeniedExportTargetDoesNotBlockPermittedTargetIntegration` |
| #100.4 UNKNOWN never causes resend/tool/model replay; child/verified_task restrictions hold | `TestCommunicationUnknownDeliveryNeverReplaysIntegration`; actual Core/Broker `private_outcome_unknown`; `internal/actionauth.TestCommunicationRequiresPinnedCompositionAndExactConsent`; `internal/budget.TestCommunicationUnknownEffectExceptionIsExactAndBoundedIntegration`; `TestCommunicationQuietAndVerifiedTaskDoNotDemandPublicationIntegration` |
| #100.5 Two publications + different/empty terminal; frozen save does not replay; memory preserves provenance | Actual Core/Broker `empty_terminal`, `different_terminal`, `frozen_save_recovery`; publication/final identity test; communication compaction/extraction tests in `internal/memory/compaction` and `internal/memory/pipeline` |
| #100.6 Missing-response/quiet work; unavailable tools and legacy deny before effects | Actual Core/Broker `quiet`, `missing_response_corrected`, `missing_response_exhausted`, `missing_response_stream_failure`; `TestCommunicationTrustedAdmissionRecoveryPinsProfileIntegration`; `TestCommunicationEpochAndExpiredWaitIntegration`; strict readiness/action authorization tests |
| #103.1 Restart/no-expiry >24h preserves question; fresh current task/session/authority required | `TestCommunicationFreshOwnershipAtEveryHandoffIntegration`; `TestCommunicationNoExpiryAndInvalidSubmitIntegration`; actual Core/Broker `widget_handoff_after_25_hours`; fresh-authority matrix |
| #103.2 Answer before initial delivery is retained; first submit creates one response/resume | `TestCommunicationFastResponseFinishAndResumeIntegration`; every-response-write matrix; multi/text response test; HTTP owner response bypasses its own waiting FIFO |
| #103.3 Both response/Finish orders, crash/lost ACK, one fresh executor and no tool replay | Every-publication/response-write process-crash matrices; fresh-ownership-at-every-handoff matrix; `orchestrator.TestCommunicationWidgetReadFailurePreservesSuspension`; `input.TestCommunicationStateReadRetryPreservesReadyAndNoExpiryWaitIntegration` |
| #103.4 Cancel/amend/expiry close old question; pause retains answer without execution until resume | `TestCommunicationTaskAmendAndStopCloseQuestionIntegration`; `TestCommunicationStopAmendAfterResponseCancelsObligationIntegration`; `TestCommunicationEpochAndExpiredWaitIntegration`; `TestCommunicationTaskPauseResponseAndFreshResumeIntegration` |
| #103.5 Taskless conversation, fixed spending/source guards, no old tool replay | Task and conversation handoff cases; actual Core/Broker widget keeps three admitted model attempts across waiting; `TestCommunicationAnswerRetainedButFreshAuthorityRequiredMatrixIntegration`; `TestCommunicationActualCatalogRevocationBlocksFreshResumeIntegration`; compaction/privacy tests |

## Fault matrix and independent assertions

`fault_matrix_test.go` wraps actual pgx transactions. It discovers and exercises
every owning Exec boundary for message/widget publication and response. The
current set has 33 SQL boundaries (including authority-check functions): each
runs once with a lost storage response and once with abrupt process exit without
deferred cleanup. Three additional exits happen after successful commit and
before ACK. The parent waits until the dead process's PostgreSQL connection is
gone before asserting rollback or durable state. This is not an uncommitted
visibility check masquerading as process recovery.

Assertions cover publication, canonical message, both immutable outbox targets,
interaction/response, ready marker, Run/Task wait, obligation and event state.
An aborted response leaves the open question and original checkpoint intact;
restart accepts one valid response. A committed operation retains the same receipt
and does not insert another transcript row or publication.

`handoff_matrix_test.go` exercises seven boundaries for both Task and taskless
conversation. Dedicated subprocesses die after fresh Input claim, Session lease,
run-lease refresh and Task activation. A replacement waits for the actual finite
leases to expire, gets new claim/fence and reconstructs a fresh model context.
Both response/Finish orders and a dead original worker are also checked. Prior
publication/correction spending stays fixed and no old tool is submitted again.

The >24h cases use controlled past Input TTL/DB time, not a claim that this test
waited 25 wall-clock hours. No-expiry wait remains durable; an answer receives a
new finite execution window. Execution authority is independently bounded.
The authority matrix creates real owner goals/mandates through their owning
services, then tests expired/revoked mandate, paused goal, stale Task revision,
lost Input claim, stale fence and source revocation. Denied resume preserves
answer DATA and rolls back Task activation. Separate real Catalog deletion
invalidates a fresh resume and removes copied question/answer data.

Delivery tests use concurrent real claims and an independently counted controlled
sender. Only confirmed absence permits retry. An actual PostgreSQL trigger rejects
the ACK write after send; a new worker quarantines the expired sending claim as
UNKNOWN and never calls that sender again. A failed/denied target cannot stop the
permitted target. Sender ACK is not human read or semantic task completion.

The app suite uses the real Core, exact private Broker gRPC subprocess, HTTP owning
control, existing approval repository, budget controller, Catalog, Input and
Completion. Only model events/channel responses are controlled. It verifies two
publications and further model continuation, different/empty terminal, frozen-save
recovery, lost ACK, unresolved UNKNOWN, quiet, one correction and failure closure,
and widget waiting. The counts come from durable effects, model attempts and
publication/final/outbox rows, not from scripted success text.

## Reproduction and result

Run `bash scripts/dev/communication-core-tests.sh --regression` on Linux with an
isolated PostgreSQL17/pgvector `*_test` database. Go1.27.2, Go caches and JSONL
evidence stay under repo `.cache/`. Required selection uses `-race -count=1` and
fails on any skip. The separately compiled Broker harness is required before
those tests run. Scoped vet and both service builds are included.

CI runs the same command in `communication-postgres`, with an ephemeral database
and retained JSONL artifact. [Current PR127 checks](https://github.com/alex123321-maker/butler/pull/127/checks)
identify the tested commit; the preceding review's green CI is not substituted.

Local validation: 60 distinct required communication tests passed across the
required race selection and final review supplements, with zero failures/skips.
The full affected regression selection passed 383 tests; 12 shared private-control
tests passed with race and zero skips. Scoped vet, both service builds and diff
checks passed. Subtests/process helpers are not counted as distinct top-level
tests, and repeated supplements are not added again. CI independently runs the
entire final selection on its current commit.

Eight unrelated regression skips are not passes:
`TestLegacyTaskMigrationIntegration` (exclusive migration DB),
`TestExecuteIntegrationPersistsRunAndTranscript` and `TestPipelineWorkerIntegration`
(Redis), `TestBrokerBrowserSequenceE2E`,
`TestResourceBrokerActualChromiumPageCASIntegration`,
`TestWebToolsBrokerSixDefectsIntegration` (browser services),
`TestDelegationBrokerHarness` (helper, exercised separately in actual subprocess
tests), `TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration`
(backup DB). The script's excluded Native/Backup/ServerBrowser names are also
outside this acceptance. No required communication case is skipped.

Final diff review checked current owner/binding/source authority, exact private
claims, immutable recipients, transaction/lock order, no second queue/sender,
UNKNOWN handling, once-only correction, terminal proof and legacy/default-off
composition. The approved-widget recovery bug and post-policy send expiry gaps
were corrected and covered. No unresolved finding remains in this backend scope;
deferred channel/product work below is explicit, not silently counted as passed.

No local or CI fixture proves live Web/Telegram, a native report
renderer, native model behavior, or production readiness. Those remain #101,
#102, #104–#107. Closing these three backend issues does not close #98 or enable
the full communication profile.
