// Package acceptance records reproducible product acceptance evidence. It does
// not execute product actions or grant permission to a model.
package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func CriteriaDigest() string {
	raw, _ := json.Marshal(struct {
		Rules     string
		Groups    []Group
		Scenarios []Scenario
		Schemas   map[string]string
	}{RulesVersion, Groups(), Scenarios(), ObservationSchemas()})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

const Module = "github.com/butler/butler/"

type Group struct {
	ID          string   `json:"id"`
	Package     string   `json:"package"`
	Runtime     string   `json:"runtime"`
	Environment []string `json:"environment,omitempty"`
	Tests       []string `json:"tests"`
}

type Scenario struct {
	ID        string   `json:"id"`
	Guarantee string   `json:"guarantee"`
	Groups    []string `json:"groups,omitempty"`
	Pending   string   `json:"pending,omitempty"`
}

// Test names are intentional: a renamed, skipped or missing test is not proof.
// Each package receives its own disposable database and executes sequentially.
func Groups() []Group {
	return []Group{
		{"ownership", "apps/orchestrator/internal/session", "go-race", []string{"BUTLER_TEST_POSTGRES_URL", "BUTLER_TEST_REDIS_URL"}, []string{"TestDurableLeaseFenceTakeoverIntegration", "TestRedisLeaseManagerIntegration"}},
		{"approval", "apps/orchestrator/internal/input", "go-race", []string{"BUTLER_TEST_POSTGRES_URL"}, []string{"TestApprovalWorkerSIGKILLRestartIntegration", "TestDurableApprovalResumesInSameProcessIntegration", "TestApprovalDecisionBeforeNotificationReturnsIntegration", "TestApprovalExpiryWithoutOwnerReplyIntegration", "TestApprovalExpiresAfterDecisionBeforeDispatchIntegration", "TestDurableApprovalRestartsSameRunAndActionIntegration", "TestDurableApprovalDecisionRacesAndInvalidationIntegration", "TestDurableUnknownEffectAndKnownResultRecoveryIntegration", "TestStaleExecutorCannotPublishDurableAuthorityIntegration", "TestApprovalCannotAuthorizeChangedSavedArgumentsIntegration", "TestTelegramDecisionIsBoundToSavedChatIntegration", "TestFinalResultRecoveryNeverRepeatsExternalCallIntegration", "TestActionSlotPersistsBeforePrepareAndSurvivesApprovalRecoveryIntegration", "TestRuntimeBudgetDeadlineCancelsIdleInferenceIntegration"}},
		{"owner-stop", "apps/orchestrator/internal/task", "go-race", []string{"BUTLER_TEST_POSTGRES_URL"}, []string{"TestControlStopPropagatesChildrenAndRetainsExternalObservations", "TestStopIdleProviderIgnoresLateFinalIntegration", "TestAmendContinuationUsesCurrentProtectedContextIntegration", "TestOwnerControlWinsBeforeRealDispatchClaimIntegration", "TestOwnerTreeStopRevokesActualGrandchildGrantIntegration", "TestRealDispatchClaimWinsBeforeOwnerControlRetainsOutcomeIntegration", "TestOwnerStopAndAtomicResultCommitOrderIntegration", "TestChildAdmissionCannotResetRootBudgetIntegration", "TestCompletionRequiresCurrentOwnerEvidenceForEveryCriterion"}},
		{"authority", "apps/orchestrator/internal/goal", "go-race", []string{"BUTLER_TEST_GOALS_POSTGRES_URL"}, []string{"TestRevocationDeniesBeforeFailedPropagationAndRestartIntegration", "TestGoalRevocationCancelsIdleProviderBeforeSweepAndIgnoresLateFinalIntegration", "TestOwnerAuthorityChangeWinsBeforeRealDispatchIntegration", "TestBudgetWaitCrossingPurposeDeadlineRollsBackEveryAdmissionIntegration", "TestExplicitOwnerTaskDoesNotNeedSpontaneousInitiativePermissionIntegration"}},
		{"effects", "internal/actionauth", "go-race", []string{"BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"}, []string{"TestEffectJournalLostAcknowledgementReconcilesWithoutReplay", "TestEffectJournalBeforeDispatchRollbackAndAbsentObservation", "TestEffectJournalGenericSuccessIsUnverifiedAndCancellationFencesClaim", "TestEffectJournalCancelAndPrepareOnlySerializeWithClaim", "TestClaimAndBudgetAdmissionAreOneTransactionIntegration", "TestUnassignedTerminalRunCannotDispatchWithStillCurrentLeaseIntegration", "TestReleaseMandateRevocationStopsBeforeCredentialInspection", "TestPublicReadNeedsExactReleaseProofAndChangedPathQueryCannotReuseIt"}},
		{"delivery", "apps/orchestrator/internal/completion", "go-race", []string{"BUTLER_TEST_COMPLETION_POSTGRES_URL", "BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestAtomicResultAndRestartIntegration", "TestAtomicRollbackAndPersistenceOnlyRecoveryIntegration", "TestOutboxRetryBoundAndImmutablePayloadIntegration", "TestOutboxChunksAndUnknownSendIntegration", "TestOutboxConcurrentClaimAndCrashIntegration", "TestSourceDeletionVersusAtomicResultAndDispatchIntegration", "TestSourceDeletionVersusLiveDispatchClaimIntegration"}},
		{"budget", "internal/budget", "go-race", []string{"BUTLER_TEST_POSTGRES_URL"}, []string{"TestSharedReservationsSurviveRestartAndRejectParallelOverspendIntegration", "TestParallelScopeAndReservationInitializationIntegration", "TestChildAndToolReservationsShareAtomicParentLimitsIntegration", "TestNormalizedLoopPersistsAcrossRestartAndTypedErrorsIntegration", "TestRuntimeAdmissionRejectsFinishedRunAndAggregateBytesIntegration", "TestLocalTargetIsIndependentOfDeclaredZeroPrice"}},
		{"context", "apps/orchestrator/internal/orchestrator", "go-race", []string{"BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestSummaryCannotHideRecentOwnerCorrection", "TestContextBudgetDropsOptionalMaterialBeforeMandatoryConstraints", "TestForeignDocumentMemoryAndToolDescriptionNeverBecomeInstructions", "TestCoveredSummaryCannotRemoveSecretLabelOrPromoteHistoricalAuthority", "TestIdleProviderCannotPublishAfterSourceDeletionIntegration", "TestOwningSourceVerificationBoundSlowCurrentAndRevocationIntegration", "TestDelegationPreparationPinsTaskBeforeSelectionAndExcludesAmbientMemory", "TestDelegationContextRejectsUnboundInstructionsSecretsAndPayloadProse", "TestFailedRunPersistsSafeTransportSubtype", "TestSourceGuardDiagnosticsAtEveryProviderEntry", "TestSourceGuardDiagnosesLateTimeoutWithoutPublishingFinal"}},
		{"security-content", "internal/security/content", "go-race", nil, []string{"TestDerivedInstructionCannotAcquireAuthority", "TestLegacyPreparedInstructionsBecomeDataAndSecretsAreDenied"}},
		{"security-egress", "internal/security/egress", "go-race", nil, []string{"TestInternalAndAmbiguousResourcesAreDenied", "TestDNSMixedAnswersAndRebindingNeverDialPrivateNetwork", "TestAllowedPublicFixtureRedirectAndByteCaps", "TestScopedProxyDeniesCrossOriginUpgradeAndCumulativeRelease", "TestCONNECTPinsDNSRejectsOtherHostsAndClosesExpiredTunnel", "TestCONNECTRejectsMixedDNSAndByteOverflow", "TestScopedProxyFreezeCancelsInflightAndPersistsUntilBoundedWindow", "TestScopedProxyFreezeClosesEstablishedCONNECT", "TestScopedProxyExpiryTimerInitializationIsSynchronized"}},
		{"security-workspace", "internal/security/workspace", "go-race", nil, []string{"TestWorkspaceDeniesSiblingSymlinkAndNonregularFiles"}},
		{"security-redaction", "internal/security/redact", "go-race", nil, []string{"TestPrivateValueCannotEscapeReflectionMaskThroughJSONEncoding"}},
		{"provider-diagnostics", "internal/transport", "go-race", nil, []string{"TestDiagnosticCodeExcludesUntrustedProviderMaterial"}},
		{"model-provenance", "internal/localmodels", "go-race", nil, []string{"TestDecisionCodeIsExactAndFailClosedBeforeTrustLoader", "TestPinnedMetadataIsBoundedAndContained", "TestProcessEnvironmentAndRawErrorAreNotExposed"}},
		{"generated-isolation", "internal/generatedtools", "go-race", nil, []string{"TestDeveloperContractCannotSmuggleDocumentOrReplaceIndependentCriteria", "TestConverterPrivateClientsNeverSelectNetworkRuntimeOrFollowControlRedirect", "TestIndependentCheckerRejectsAuthorMappingAndAcceptanceChanges", "TestPackageRejectsExecutableAuthorityAndDuplicateMembers", "TestDeveloperFreshTemporaryWorkcopyRemoved"}},
		{"delegated-sources", "internal/memory/catalog", "go-race", []string{"BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestReadDelegatedRunDerivesOnlySelectedCurrentWorkerContextIntegration", "TestReadDelegatedRunBoundsBodiesAndDeniesStoppedWorkerIntegration", "TestDelegatedContextSelectsBodiesWithoutAmbientProfileOrHiddenAncestorIntegration", "TestDelegationProposalChecksParentGrantEvenWithOwnerBodyAccessIntegration", "TestDelegatedSourceGrantRevocationAndImmutablePinsIntegration"}},
		{"memory", "internal/memory/catalog", "go-race", []string{"BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestSourceDuplicateAndConflictingPreferencesIntegration", "TestConcurrentExplicitSupersessionIntegration", "TestTransitiveDeletionAndBackupRestoreIntegration", "TestMissingManifestFailsClosedIntegration", "TestMatchingWatermarkCannotHideWrongDeletionHistoryIntegration", "TestPrivacyCleanupRestartAndStaleSummaryWorkingPublicationIntegration", "TestDeletionRedactsImmutableFinalWithoutInventingUndoIntegration", "TestLiveSourceAndTranscriptPinsRejectLatePublicationIntegration", "TestDeletionAnchorRejectsSymlinkReplacement"}},
		{"source-index", "internal/memory/semantic", "go-race", []string{"BUTLER_TEST_SEMANTIC_POSTGRES_URL", "BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestSemanticIndexPostgresReindexRollbackAndSourceChanges", "TestCitationChangeReusesVectorWithoutNewInference", "TestSourceCorrectionCommitsBeforeWaitingVectorPersistence", "TestSecretSourceExcludedFromIndexAndModelAdmission", "TestOwnedJobRebuildPinsSourcesChargesEveryAttemptIntegration", "TestOwnedSourceSnapshotRejectsCorrectionBeforeAndAfterInferenceIntegration", "TestSecretOriginDeniesBeforeModelIPC"}},
		{"auxiliary", "internal/memory/jobs", "go-race", []string{"BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestOwnedAuxiliaryAfterRunAndClaimTakeoverIntegration", "TestEachActualAuxiliaryAttemptChargedAndBoundedIntegration", "TestTaskAmendmentImmediatelyDeniesPostRunAuxiliaryIntegration", "TestForgedAuxiliarySnapshotsCannotExpandAccessOrAllowanceIntegration", "TestAuxiliaryManifestPinsOriginalAncestorsIntegration"}},
		{"model-pressure", "internal/localmodels", "go-race", nil, []string{"TestOOMRetriesWholeInputThenCPU", "TestFullDevicePressureCancelsGPUAndFallsBack", "TestUnknownResidentNeverEvicted", "TestIdleUnloadAndRestartLeavesNoFalseSuccess", "TestRAMReservationsAreSharedAcrossLanesAndReleased", "TestRAMOverrunCancelsOwnedCPUWorkAndUnloads", "TestProcessCancellationKillsAndWaitsForOwnedRuntime"}},
		{"extensions", "internal/extensions", "go-race", []string{"BUTLER_TEST_POSTGRES_URL"}, []string{"TestReviewedLifecycleKeepsStatesHistoryAndVersionPinsIntegration", "TestConcurrentProbeAdmissionAndInterruptedHealthIntegration", "TestExtensionCatalogAndDispatchUseCurrentPermissionIntegration", "TestPermissionRevocationAndTaskAmendDenySavedActionIntegration", "TestExtensionDisableLinearizesWithGrantClaimIntegration", "TestReviewedReceiptIsExactContractAndGenericWriteStaysUnknownIntegration"}},
		{"browser", "apps/tool-browser/internal/runtime", "chromium", []string{"BUTLER_BROWSER_E2E"}, []string{"TestBrowserSessionE2E", "TestSessionGuardsBeforeEffect", "TestExclusiveControlRejectsConcurrentEffect", "TestProcessFailureInvalidatesAllSessions", "TestCredentialArgumentsRequireMatchingBrokerResolution", "TestCookieScopeCheckedBeforeSecretInjection"}},
		{"broker-browser", "apps/tool-broker/internal/broker", "chromium", []string{"BUTLER_BROWSER_E2E", "BUTLER_BROWSER_E2E_BINARY"}, []string{"TestBrokerBrowserSequenceE2E", "TestDirectBrokerCallsCannotBypassActionAuthorization", "TestReflectedCredentialIsMaskedBeforeModelBoundary"}},
		{"credential-e2e", "apps/orchestrator/internal/app", "chromium", []string{"BUTLER_TEST_ACTIONAUTH_POSTGRES_URL", "BUTLER_ACTIONAUTH_E2E", "BUTLER_BROWSER_E2E_BINARY", "BUTLER_BROKER_E2E_BINARY"}, []string{"TestApprovedCredentialBrokerBrowserE2E"}},
		{"extension-e2e", "apps/orchestrator/internal/app", "process", []string{"BUTLER_TEST_ACTIONAUTH_POSTGRES_URL", "BUTLER_EXTENSION_E2E", "BUTLER_EXTENSION_E2E_BINARY", "BUTLER_BROKER_E2E_BINARY"}, []string{"TestInstalledProjectExtensionRealBrokerApprovalE2E"}},
		{"backup-seed", "internal/memory/catalog", "backup", []string{"BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL", "BUTLER_TEST_MEMORY_BACKUP_PHASE", "BUTLER_TEST_MEMORY_BACKUP_MANIFEST"}, []string{"TestActualPostgresDumpRestoreDeletionIntegration"}},
		{"backup-delete", "internal/memory/catalog", "backup", []string{"BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL", "BUTLER_TEST_MEMORY_BACKUP_PHASE", "BUTLER_TEST_MEMORY_BACKUP_MANIFEST"}, []string{"TestActualPostgresDumpRestoreDeletionIntegration"}},
		{"backup-restore", "internal/memory/catalog", "backup", []string{"BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL", "BUTLER_TEST_MEMORY_BACKUP_PHASE", "BUTLER_TEST_MEMORY_BACKUP_MANIFEST"}, []string{"TestActualPostgresDumpRestoreDeletionIntegration"}},
	}
}

func Scenarios() []Scenario {
	return []Scenario{
		{"browser-observe-act", "Actual Chromium: one session, multi-step form, stale page and exclusive control before effect.", []string{"browser", "broker-browser", "credential-e2e"}, ""},
		{"approval-restart", "Persisted approval restart, reply-before-notification, duplicate/stale decisions and exact arguments.", []string{"approval", "ownership", "effects"}, ""},
		{"stop-and-ownership", "Owner restriction/stop fences current and descendant dispatch; late final cannot publish.", []string{"owner-stop", "authority", "ownership"}, ""},
		{"effect-and-delivery", "Lost acknowledgement remains unknown, result recovery and delivery retry never repeat an external action.", []string{"effects", "delivery", "approval"}, ""},
		{"fresh-context-and-deletion", "Current correction beats summary; task-owned context, transitive redaction, dump/restore watermark and guarded reindex.", []string{"context", "owner-stop", "memory", "source-index", "backup-seed", "backup-delete", "backup-restore"}, ""},
		{"shared-cost-and-loop", "Persisted shared quotas, parallel reservations and normalized loop stop; no lease borrowed by auxiliary work.", []string{"budget", "auxiliary", "source-index"}, ""},
		{"delegation-conflict", "Real delegated workers contest tab/file, stop all branches, and compare total single/multi-agent cost.", nil, "Requires implemented #16/#17 delegation/resource ownership. Task-tree fixtures alone do not prove actual delegated execution."},
		{"untrusted-content", "Document/tool/selected-child output stays DATA through derivation and fresh recovery; secret is denied before IPC; unauthorized query/resource cannot reuse approval; reviewed code, filesystem and egress boundaries remain independent of model decisions.", []string{"context", "security-content", "security-egress", "security-workspace", "security-redaction", "provider-diagnostics", "model-provenance", "generated-isolation", "delegated-sources", "source-index", "effects", "credential-e2e", "extensions", "extension-e2e", "memory"}, ""},
		{"model-memory-pressure", "Controlled worker OOM/whole-device pressure, bounded fallback, cancellation and no foreign resident eviction.", []string{"model-pressure"}, "Real hardware OOM comparison is not represented by controlled worker fixtures; model-observation resource measurements are separate."},
		{"installed-extension-boundaries", "Current installed version/readiness/task permission, interrupted probe and disable-versus-grant order; actual private Broker and HTTP extension runtime with mandatory transcript pins.", []string{"extensions", "extension-e2e"}, ""},
		{"extension-release-rollback", "Incompatible installed version, failed readiness, independently approved release and rollback.", []string{"extensions", "extension-e2e"}, "Installed lifecycle is covered; full isolated development/self-repair release scenarios still require #20/#21."},
		{"russian-model-evaluation", "Versioned Russian search/extraction/classification/commands with independent holdout and observation-only model outputs.", nil, "Corpus validation is not model accuracy. Current corpus needs an independently signed golden approval and actual pinned-model observations."},
		{"full-release-chain", "Mandate → problem → task → delegation → isolated adapter development → independent tests → approved connection → verified result → one notification, repeated across restart/stop/delivery failure.", nil, "Requires #15/#16/#17/#19/#20/#21 integration; unavailable steps remain pending."},
	}
}
