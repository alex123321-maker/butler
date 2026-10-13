package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const FullFaultVersion = "butler-mandatory-faults-v3"

// V1 Groups/Scenarios/CriteriaDigest are also the independently signed V3
// model-quality criteria. Keep them immutable. These current product fault
// checks are an additional gate, never a replacement quality approval.
func FullFaultGroups() []Group {
	groups := Groups()
	for i := range groups {
		switch groups[i].ID {
		case "owner-stop":
			groups[i].Tests = append(groups[i].Tests, "TestControlAmendStopsOldApprovalAndPreservesNewContext", "TestDelegationBoundedParallelBudgetAndAdmissionRollbackIntegration", "TestDelegationTotalBranchCapAndMissingAuthorityIntegration", "TestDelegationDependencyCyclesConflictsAndSourceRevocationIntegration", "TestDelegationStaleParentSubsetDepthAndStopRaceIntegration")
		case "context":
			groups[i].Tests = append(groups[i].Tests, "TestDurableTaskExecutionIntegration", "TestDelegationRecoveryRechecksSelectionAndPreservesUnknownEffect", "TestModelTurnUsageOwnsOnePGObservationAcrossToolsFinalsAndFailuresIntegration")
			groups[i].Environment = append(groups[i].Environment, "BUTLER_TEST_POSTGRES_URL")
		case "extensions":
			groups[i].Tests = append(groups[i].Tests, "TestProbeCASQuotaAndRestartNeverReplayIntegration")
		}
	}
	return append(groups,
		Group{"resources", "internal/resources", "go-race", []string{"BUTLER_TEST_POSTGRES_URL"}, []string{"TestResourceReadersAndExclusiveWriterAcrossSessionsIntegration", "TestResourceSetAtomicFailureAndOppositeOrderIntegration", "TestResourceExpiredWriterRemainsBlockedUntilEvidenceReconciliationIntegration", "TestResourceCurrentTaskParentAndOwnerFencesIntegration", "TestResourceConfirmedWriteConsumesLeaseAndInvalidatesObservationIntegration", "TestResourceDeadlineIsRecheckedAfterContendedLockIntegration"}},
		Group{"workspace", "internal/workspace", "go-race", []string{"BUTLER_TEST_POSTGRES_URL", "BUTLER_TEST_MEMORY_POSTGRES_URL", "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"}, []string{"TestResourceWorkspaceIndependentLeaseContendersIntegration", "TestResourceWorkspaceTwoContendersAndStaleObservationsIntegration", "TestResourceWorkspaceUnknownClaimsSurviveFailureAndNeedOwnerEvidenceIntegration", "TestPhysicalDriftAndFailedMetadataCommitBlockWithoutReplayIntegration", "TestConcurrentRealPatchesCannotOverwriteAcrossStoreInstancesIntegration"}},
		Group{"workspace-runtime", "internal/workspaceexec", "go-race", []string{"BUTLER_TEST_POSTGRES_URL", "BUTLER_TEST_MEMORY_POSTGRES_URL", "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"}, []string{"TestWorkspaceDispatchClaimAtomicallyBindsInputBudgetSourceAndResourceIntegration", "TestWorkspaceRuntimeCloseDrainsActualExecuteAndKeepsUnknownLeaseIntegration", "TestWorkspaceInstalledReadinessRejectsWrongProfileRootAndDisabledHistoryIntegration", "TestWorkspaceRuntimeActualPrivateRPCAndPhysicalConfirmationIntegration", "TestWorkspaceOwnerDiscardActualRuntimeProcessLossIntegration", "TestWorkspaceOwnerDiscardLostErasureAcknowledgmentObservesOnlyIntegration", "TestWorkspaceOwnerDiscardSupersedesOnlyStaleUnattemptedIntentIntegration"}},
		Group{"delegation-reports", "apps/orchestrator/internal/app", "go-race", []string{"BUTLER_TEST_MEMORY_POSTGRES_URL"}, []string{"TestDelegationConflictingReportsRequireOriginalWitnessIntegration"}},
		Group{"single-tab", "apps/tool-broker/internal/broker", "chromium", []string{"BUTLER_SINGLE_TAB_E2E", "BUTLER_SINGLE_TAB_HOST_BINARY", "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"}, []string{"TestSingleTabResourceActualChromiumNativeHostIntegration", "TestSingleTabResourceActualChromiumNativeHostIntegration/valid", "TestSingleTabResourceActualChromiumNativeHostIntegration/manual_input", "TestSingleTabResourceActualChromiumNativeHostIntegration/manual_new_observation", "TestSingleTabResourceActualChromiumNativeHostIntegration/dom_mutation", "TestSingleTabResourceActualChromiumNativeHostIntegration/navigation", "TestSingleTabResourceActualChromiumNativeHostIntegration/foreign_origin", "TestSingleTabResourceActualChromiumNativeHostIntegration/foreign_host", "TestSingleTabResourceActualChromiumNativeHostIntegration/lost_ack_restart", "TestSingleTabResourceActualChromiumNativeHostIntegration/two_executors", "TestSingleTabResourceActualChromiumNativeHostIntegration/installed_native"}},
	)
}

func FullFaultScenarios() []Scenario {
	return []Scenario{
		{"browser-observe-act", "Actual Chromium multi-step session and current observation, page/tab drift and lost acknowledgement.", []string{"browser", "broker-browser", "credential-e2e", "single-tab"}, ""},
		{"approval-restart", "Persisted restart, reply-before-notification, double/stale decisions and changed arguments.", []string{"approval", "ownership", "effects"}, ""},
		{"stop-and-ownership", "Current owner 'do not send' amendment, stopped descendants and lost/exclusive resource ownership.", []string{"owner-stop", "authority", "ownership", "resources", "workspace-runtime"}, ""},
		{"effect-and-delivery", "Before/after-effect disconnect, UNKNOWN and delivery recovery without repeating the original action.", []string{"effects", "delivery", "approval", "workspace", "workspace-runtime", "single-tab"}, ""},
		{"fresh-context-and-deletion", "Fresh constraints beat old summary; a fresh task survives old failure; deletion/reindex and actual dump/restore deny old data.", []string{"context", "memory", "source-index", "backup-seed", "backup-delete", "backup-restore"}, ""},
		{"shared-cost-and-loop", "Actual tab/file contenders and independently checked conflicting child reports, durable shared budget, depth/branch caps, dependency cycles and normalized loops.", []string{"resources", "workspace", "single-tab", "budget", "owner-stop", "delegation-reports"}, ""},
		{"untrusted-content", "External/child prose stays data; synthetic secrets and unallowed resources are denied before authority/effects.", []string{"context", "delegated-sources", "security-content", "security-egress", "security-workspace", "security-redaction", "generated-isolation", "credential-e2e"}, ""},
		{"model-and-release-failure", "Actual isolated cached CPU model LOAD OOM before inference; separately tested owned-worker cancellation/no replay and reviewed incompatible/failed extension release/rollback.", []string{"model-load-oom", "model-pressure", "model-provenance", "extensions", "extension-e2e", "workspace-runtime"}, ""},
	}
}

func FaultCriteriaDigest() string {
	raw, _ := json.Marshal(struct {
		Version                   string
		Groups                    []Group
		Scenarios                 []Scenario
		OOM                       string
		ApprovedVocabularyWitness any
	}{FullFaultVersion, FullFaultGroups(), FullFaultScenarios(), LocalLoadOOMVersion, approvedLocalLoadRules()})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type FullFaultReport struct {
	Version                     string            `json:"version"`
	CreatedUTC                  time.Time         `json:"created_utc"`
	CodeCommit                  string            `json:"code_commit"`
	SourceTreeSHA256            string            `json:"source_tree_sha256"`
	SchemaSHA256                string            `json:"schema_sha256"`
	CriteriaSHA256              string            `json:"criteria_sha256"`
	SignedQualityCriteriaSHA256 string            `json:"signed_quality_criteria_sha256"`
	Hardware                    json.RawMessage   `json:"hardware"`
	MandatoryFaultsPassed       bool              `json:"mandatory_faults_passed"`
	Issue33Ready                bool              `json:"issue_33_ready"`
	Groups                      []GroupProof      `json:"groups"`
	Scenarios                   []ScenarioProof   `json:"scenarios"`
	LocalModelLoadOOM           *LocalLoadOOM     `json:"local_model_load_oom,omitempty"`
	LocalModelLoadWitness       *LocalLoadWitness `json:"local_model_load_witness,omitempty"`
	MeasurementLimits           []string          `json:"measurement_limits"`
}

type faultRunPin struct {
	Version          string `json:"version"`
	CodeCommit       string `json:"code_commit"`
	SourceTreeSHA256 string `json:"source_tree_sha256"`
	SchemaSHA256     string `json:"schema_sha256"`
	CriteriaSHA256   string `json:"criteria_sha256"`
	HardwareSHA256   string `json:"hardware_sha256"`
}

// WriteFaultRunPin is called before any selected test starts. Later report mode
// verifies this initial release/hardware identity rather than silently blessing
// events from a different source release. Event files themselves stay O_EXCL.
func WriteFaultRunPin(root, events, commit string, hardware json.RawMessage) error {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) || !json.Valid(hardware) {
		return fmt.Errorf("exact release/hardware required")
	}
	source, err := TreeHash(root, nil)
	if err != nil {
		return err
	}
	schema, err := TreeHash(root, func(s string) bool { return strings.HasPrefix(s, "migrations/") })
	if err != nil {
		return err
	}
	hash := sha256.Sum256(hardware)
	if err = os.MkdirAll(events, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(events, "fault-run-pin.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(faultRunPin{FullFaultVersion, commit, source, schema, FaultCriteriaDigest(), hex.EncodeToString(hash[:])})
}

func BuildFullFaultReport(root, events, commit string, hardware json.RawMessage, oomPath string) (FullFaultReport, error) {
	r := FullFaultReport{Version: FullFaultVersion, CreatedUTC: time.Now().UTC(), CodeCommit: commit, Hardware: hardware, CriteriaSHA256: FaultCriteriaDigest(), SignedQualityCriteriaSHA256: CriteriaDigest(), MandatoryFaultsPassed: true}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) || !json.Valid(hardware) {
		return r, fmt.Errorf("exact commit and hardware manifest required")
	}
	var err error
	if r.SourceTreeSHA256, err = TreeHash(root, nil); err != nil {
		return r, err
	}
	if _, err = os.Stat(filepath.Join(root, "migrations")); err != nil {
		return r, fmt.Errorf("migration source required")
	}
	if r.SchemaSHA256, err = TreeHash(root, func(s string) bool { return strings.HasPrefix(s, "migrations/") }); err != nil {
		return r, err
	}
	states := map[string]string{}
	pinProof := GroupProof{ID: "release-pin", Runtime: "pre-test-release-manifest", Status: "FAIL", Reason: "initial exact release/hardware pin missing or mismatched"}
	if raw, readErr := ReadPrivateFile(filepath.Join(events, "fault-run-pin.json"), 4096); readErr == nil {
		var pin faultRunPin
		hash := sha256.Sum256(hardware)
		if DecodeStrict(raw, &pin, "version", "code_commit", "source_tree_sha256", "schema_sha256", "criteria_sha256", "hardware_sha256") == nil && pin.Version == FullFaultVersion && pin.CodeCommit == commit && pin.SourceTreeSHA256 == r.SourceTreeSHA256 && pin.SchemaSHA256 == r.SchemaSHA256 && pin.CriteriaSHA256 == r.CriteriaSHA256 && pin.HardwareSHA256 == hex.EncodeToString(hash[:]) {
			pinProof.Status, pinProof.Reason = "PASS", ""
			pinProof.EventsSHA256, _ = FileHash(filepath.Join(events, "fault-run-pin.json"))
		}
	}
	r.Groups = append(r.Groups, pinProof)
	for _, g := range FullFaultGroups() {
		p := GroupProof{ID: g.ID, Runtime: g.Runtime, Status: "FAIL", Reason: "required current event artifact missing"}
		path := filepath.Join(events, g.ID+".jsonl")
		if f, openErr := os.Open(path); openErr == nil {
			p.Tests, err = VerifyEvents(f, g)
			f.Close()
			p.EventsSHA256, _ = FileHash(path)
			p.Status, p.Reason = "PASS", ""
			if err != nil {
				p.Status, p.Reason = "FAIL", err.Error()
			}
			for _, test := range p.Tests {
				if test.Status != "PASS" {
					p.Status = "FAIL"
				}
			}
		}
		states[p.ID] = p.Status
		r.Groups = append(r.Groups, p)
	}
	oom := GroupProof{ID: "model-load-oom", Runtime: "actual-cached-cpu-model-load", Status: "FAIL", Reason: "required local model-load OOM artifact missing"}
	if oomPath != "" {
		oom.EventsSHA256, _ = FileHash(oomPath)
		if proof, proofErr := ReadLocalLoadOOM(root, oomPath); proofErr == nil {
			r.LocalModelLoadOOM = &proof
			oom.Status, oom.Reason = "PASS", ""
			oom.EventsSHA256, _ = FileHash(oomPath)
		} else {
			oom.Reason = proofErr.Error()
			// Retain failed physical observations only when the immutable identity
			// and isolation match. A missing metadata witness stays a failed gate;
			// known exit/OOM/attempt measurements do not become unavailable or0.
			if !proof.ModelMetadataRead && validateLocalLoadExperiment(root, proof) == nil {
				r.LocalModelLoadOOM = &proof
			}
			if witness, witnessErr := ReadApprovedLocalLoadWitness(root, oomPath); witnessErr == nil {
				r.LocalModelLoadWitness = &witness
				oom.Status, oom.Reason = "PASS", "independently reviewed vocabulary-metadata witness; original strict report remains FAIL"
			}
		}
	}
	states[oom.ID] = oom.Status
	r.Groups = append(r.Groups, oom)
	for _, scenario := range FullFaultScenarios() {
		p := ScenarioProof{Scenario: scenario, Status: "PASS"}
		for _, group := range scenario.Groups {
			if states[group] != "PASS" {
				p.Status = "FAIL"
				r.MandatoryFaultsPassed = false
			}
		}
		r.Scenarios = append(r.Scenarios, p)
	}
	// All groups are mandatory even if a future edit accidentally leaves an
	// otherwise required group out of a scenario mapping.
	for _, group := range r.Groups {
		if group.Status != "PASS" {
			r.MandatoryFaultsPassed = false
		}
	}
	r.MeasurementLimits = []string{"Synthetic external effects/channels and actual isolated Chrome/PG/backup tests do not measure general model quality.", "Cached Qwen3 CPU LOAD OOM is before inference. It does not observe Antigravity 3.8 Flash High server RAM/OOM or establish that Butler's kernel received that physical OOM. Owned-worker/kernel failure controls are separate tests.", "Independently reviewed High task-quality/cost observations and the complete mandate-to-one-final-notice release chain, including its restart/Stop/delivery faults, are separate required gates. This fault-only report never sets issue_33_ready.", "Historical V1 matrix, signed V3 quality digest/corpus and all old failed/native reports remain immutable."}
	return r, nil
}
