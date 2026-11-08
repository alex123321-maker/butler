package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

type systemGroup struct {
	ID, Package string
	Tests       []string
}

var systemGroups = []systemGroup{
	{"sessions", "apps/orchestrator/internal/session", []string{"TestCreateSessionIsIdempotent", "TestResolveSessionKeyPrefersChatID"}},
	{"run-lifecycle", "apps/orchestrator/internal/run", []string{"TestCreateRunReturnsExistingRunAfterDuplicateConflict", "TestTransitionRunRejectsInvalidTransition"}},
	{"single-tab", "internal/singletabcontrol", []string{"TestGuardedRequestPrivateContract", "TestSingleTabResourcePrivateMetadataHTTP"}},
	{"extensions", "internal/extensions", []string{"TestManifestIsImmutableDataAndHasNoExecutionHook", "TestManifestRejectsAmbiguousOrUnboundedCapabilities"}},
	{"audio-input", "internal/audio", []string{"TestCanonicalAudioBounds"}},
	{"local-models", "internal/localmodels", []string{"TestStructuredExtractionPreservesUnicodeAndRejectsFabricatedFields", "TestCleanCacheCreditDoesNotOvercommitConcurrentLanes"}},
	{"transport-openai", "internal/transport/openai", []string{"TestInstructionsSurviveToolCompletionAndEndWithRun", "TestSubmitToolResultUsesFunctionCallOutputInput"}},
	{"transport-codex", "internal/transport/openaicodex", []string{"TestSubmitToolResultReplaysFunctionCallStateless"}},
	{"transport-copilot", "internal/transport/githubcopilot", []string{"TestSequentialToolCallsQueueLocallyBeforeResume"}},
	{"transport-gemini", "internal/transport/antigravity", []string{"TestFlashHighCloudCodeToolContinuation", "TestToolTurnUsageTruncatedFunctionFailureRetainsOnlyCompleteMeasurements"}},
	{"transport-ollama", "internal/transport/ollama", []string{"TestPinnedRequestAndUsage", "TestUsageAbsenceDoesNotBecomeZero"}},
	{"telegram-delivery", "apps/orchestrator/internal/deliveryoutbox", []string{"TestTelegramChunksPreserveUnicodeAndUTF16Limit"}},
	{"catalog", "internal/toolcatalog", []string{"TestToolSearchRussianAndExactRefs", "TestToolSearchBeyond64PaginationAndStaleCursor"}},
	{"web-documents", "apps/tool-webfetch/internal/runtime", []string{"TestReadableTextExcludesScriptAndStyle", "TestDocumentLinksRetainPrimaryReferencesWithoutPromotingTrust", "TestExtractRefusesUnsuccessfulHTTPStatus"}},
	{"http", "apps/tool-http/internal/runtime", []string{"TestExecutePerformsHTTPRequest", "TestNoSelectorMatchIsEmptyArray", "TestUnsupportedSelectorRefusedBeforeNetwork"}},
	{"doctor", "apps/tool-doctor/internal/runtime", []string{"TestCheckContainerUsesConfiguredHealthEndpoint", "TestCheckerCheckProviderUsesHTTPProbe"}},
	{"browser-pages", "internal/browserpage", []string{"TestPageContractRejectsAmbiguousOrCredentialInputs"}},
	{"browser-sso", "internal/browsersso", []string{"TestSSOConsentDigestPinsGenerationAndAccount", "TestSSOExecutionStrictReferencesAndNetworkCaps"}},
	{"browser-spa", "internal/browserspa", []string{"TestProfileArgsAndDestinationCanonicalization", "TestReadProofDoesNotExportAContextOrClaimCommit", "TestQueryProfileGrammar", "TestSocketReadStateMachineRejectsReplayMutationAndAuthPayload"}},
	{"browser-recipient", "internal/browserspa/eval", []string{"TestPublicDocumentDoesNotContainExpectedValues"}},
	{"browser-network", "internal/security/egress", []string{"TestProxySSEFlushAndFreeze", "TestProxyReconnectAllowanceSurvivesFreezeAndThaw"}},
	{"credentials-approvals", "internal/actionauth", []string{"TestApprovalBindingRejectsChangedIntent", "TestCredentialRefsMustMatchExactArgs", "TestOpaqueBrowserEffectsRequireExplicitAuthority"}},
	{"memory", "internal/memory/readcontrol", []string{"TestStrictReadBoundsAndIdentity", "TestMessageContractsAndBounds"}},
	{"dated-facts", "internal/memory/factcontrol", []string{"TestTemporalFactsStrictSourceDateAndBoundContract"}},
	{"mobile", "internal/mobile", []string{"TestReplayHashIncludesPayloadAndMetadata", "TestMessagePartsAndNarrowNoise"}},
	{"schedules", "internal/scheduling", []string{"TestOneShotAndSelectedWeekdays", "TestScheduleSelectedWeekdaySkipsMissingDSTWeek"}},
	{"initiative", "apps/orchestrator/internal/initiative", []string{"TestDowntimeCoalescesOnceAndKeepsIntervalPhase", "TestObservationFiltersBeforeAnyModelOrTaskAdmission"}},
	{"workspace", "internal/workspace", []string{"TestModelJSONCannotSupplyPrivateWorkspaceAuthority"}},
	{"skills", "internal/skills", []string{"TestStrictManifestAndIndependentEvidence", "TestCorrectionCannotChangeProcedureOrReplaySafety"}},
	{"delegation", "internal/delegation", []string{"TestDelegatedPermissionCannotExpandAnyAuthorityDimension", "TestNumericObservationChecksOriginalInsteadOfAgreement"}},
	{"converters", "internal/generatedtools", []string{"TestConverterExactValuesBoundedErrorsAndNoPartialOutput", "TestIndependentCheckerRejectsAuthorMappingAndAcceptanceChanges"}},
	{"communication", "internal/communicationcontrol", []string{"TestCommunicationStrictIntentAndBlockingQuestions", "TestCommunicationTypedAnswers"}},
	{"budgets", "internal/budget", []string{"TestConversationModelTurnOptOutRetainsOtherBounds", "TestModelRetryOperationWitnessIsRequestLocal"}},
	{"orchestration", "apps/orchestrator/internal/orchestrator", []string{"TestExecuteRunsSequentialToolLoop", "TestExecuteToolWithApprovalRejected", "TestToolDiscoveryModelLoopSearchLoadCallFinal", "TestToolDiscoveryDurableSelectionRecoveryAndRevocation", "TestSummaryCannotHideRecentOwnerCorrection", "TestCommunicationRequiresFinalAndTerminalAndSuppressesDeltas"}},
}

type systemResult struct {
	Started    time.Time              `json:"started_utc"`
	DurationUS int64                  `json:"command_us"`
	Feature    string                 `json:"feature"`
	Package    string                 `json:"package"`
	Status     string                 `json:"status"`
	Tests      []acceptance.TestProof `json:"tests"`
	Reason     string                 `json:"reason,omitempty"`
}

func system(ctx context.Context, root, output string) error {
	if _, err := os.Stat(filepath.Join(root, "configs", "tools.json")); err != nil {
		return fmt.Errorf("run from Butler repository root")
	}
	report := struct {
		Started    time.Time      `json:"started_utc"`
		DurationUS int64          `json:"total_us"`
		Passed     int            `json:"passed"`
		Planned    int            `json:"planned"`
		SuiteSHA   string         `json:"selection_sha256"`
		Version    string         `json:"version"`
		Revision   string         `json:"checkout_revision"`
		ModelCalls int            `json:"model_calls"`
		Groups     []systemResult `json:"groups"`
	}{Version: version, Started: time.Now().UTC(), SuiteSHA: digest(systemGroups)}
	for _, g := range systemGroups {
		report.Planned += len(g.Tests)
	}
	start := time.Now()
	commit, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	report.Revision = strings.TrimSpace(string(commit))
	passed, total := 0, 0
	for _, g := range systemGroups {
		command := exec.CommandContext(ctx, "go", "test", "-json", "-count=1", "-timeout=3m", "-run", "^("+strings.Join(g.Tests, "|")+")$", "./"+g.Package)
		command.Dir = root
		command.Stderr = os.Stderr
		groupStart := time.Now()
		raw, runErr := command.Output()
		elapsed := time.Since(groupStart)
		proofs, checkErr := acceptance.VerifyEvents(bytes.NewReader(raw), acceptance.Group{Package: g.Package, Tests: g.Tests})
		row := systemResult{Feature: g.ID, Package: g.Package, Status: "pass", Tests: proofs, Started: groupStart.UTC(), DurationUS: elapsed.Microseconds()}
		skipped := map[string]bool{}
		for line := range bytes.Lines(raw) {
			var event acceptance.TestEvent
			if json.Unmarshal(line, &event) == nil && event.Action == "skip" {
				name, _, _ := strings.Cut(event.Test, "/")
				skipped[name] = true
			}
		}
		if runErr != nil || checkErr != nil {
			row.Status = "error"
			row.Reason = "package did not complete successfully"
		}
		for i := range proofs {
			if skipped[proofs[i].Name] {
				proofs[i].Status = "FAIL"
				proofs[i].Reason = "required test or subtest skipped"
			}
			if proofs[i].Status != "PASS" {
				row.Status = "fail"
			} else if runErr == nil && checkErr == nil {
				passed++
			}
		}
		total += len(g.Tests)
		report.Groups = append(report.Groups, row)
		fmt.Fprintf(os.Stderr, "%s  %s\n", row.Status, g.ID)
		if ctx.Err() != nil {
			break
		}
	}
	report.Passed = passed
	report.DurationUS = time.Since(start).Microseconds()
	if err := writeJSON(output, report); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d/%d named system checks passed\n", passed, total)
	if passed != total || len(report.Groups) != len(systemGroups) {
		return fmt.Errorf("system checks incomplete or failed")
	}
	return nil
}
