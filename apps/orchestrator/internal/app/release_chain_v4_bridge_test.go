package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

// Synthetic metadata tests the bridge predicates only. It is not a model,
// approved artifact, SQL provenance, independent quality verdict or admission.
func syntheticReleaseChainV4Bridge(t *testing.T) (releaseChainV4Prerequisite, nativeHistoricalEvidence, acceptance.CloudV4WindowAudit, acceptance.CloudV4PGWitness, acceptance.CloudV4PGCorrelation) {
	t.Helper()
	history, err := readScenarioHighV4History()
	if err != nil {
		t.Fatal("immutable prior268 unavailable")
	}
	raw, err := readShadowV4CorpusBytes(filepath.Join(shadowRepoRoot(), "testdata", "acceptance", "ru-cloud-fresh-v4.json"))
	var corpus acceptance.CloudV4Corpus
	if err != nil || json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("exact current candidate unavailable")
	}
	p := releaseChainV4Prerequisite{Version: "butler-chain-high-reviewed-v4-prerequisite-v1", PriorHistorySHA: history.BaselineSHA256,
		ReportSHA: strings.Repeat("1", 64), AuditSHA: strings.Repeat("2", 64), PGSHA: strings.Repeat("3", 64), CorrelationSHA: strings.Repeat("4", 64), Code: strings.Repeat("5", 40), SourceSHA: strings.Repeat("6", 64), HardwareSHA: strings.Repeat("7", 64),
		NewAttempts: 252, Baseline: 520, RequestedModel: acceptance.CloudRequestedModel, ObservedVersion: "gemini-3.8-flash-v4review", GenerationTokens: 32768}
	pin := acceptance.CloudPin{Mode: "shadow_observation", Provider: acceptance.CloudProvider, EvidenceKind: "native_cloud_observation", WindowID: acceptance.CloudV4AuditedWindow,
		CodeCommit: p.Code, SourceSHA256: p.SourceSHA, HardwareSHA256: p.HardwareSHA, RequestedModel: p.RequestedModel,
		CorpusDigest: acceptance.CloudV4CorpusDigest(corpus), PolicySHA256: acceptance.CloudV4PolicyDigest(corpus), SchemaSHA256: acceptance.CloudV4SchemaDigest(), ResourcesSHA256: acceptance.CloudV4ResourcesDigest()}
	a := acceptance.CloudV4WindowAudit{Status: "CLOUD_V4_WINDOW_AUDIT_PASS", EvidenceKind: "offline_v4_window_audit", CollectorStatus: "passed", OriginalReportSHA256: p.ReportSHA, Pin: pin,
		HistorySHA256: p.PriorHistorySHA, HistoricalAttempts: 268, HistoricalArtifacts: 19, HistoricalKnown: 242, HistoricalUnknown: 26, HistoricalInput: 155423, HistoricalOutput: 5943, HistoricalTotal: 273072, HistoricalNullTotal: 46,
		NewAttempts: 252, CarryForwardAttempts: 520, MaximumNew: 252, MaximumTotal: 520, Expires: time.Now().Add(30 * time.Minute), CorrelationValid: true, QualityValid: true, ComparisonValid: true, ModelFamily: "observed_3.8_flash_family",
		Cost:       acceptance.CloudCost{Attempts: 252, FullTokenCostKnown: true, KnownInputTokens: 252, KnownOutputTokens: 252, KnownTotalTokens: 756},
		Quality:    acceptance.CloudV4QualityReport{Status: "CLOUD_V4_SHADOW_THRESHOLDS_PASS", Complete: true, ThresholdsPassed: true, Cost: acceptance.CloudCost{FullTokenCostKnown: true}},
		Comparison: acceptance.CloudComparisonReport{Status: "CLOUD_V4_SHADOW_COMPARISON_PASS", PairedTasks: 4, SingleCorrect: 4, MultiCorrect: 4, ThresholdsPassed: true, FullTokenCostKnown: true}}
	w := acceptance.CloudV4PGWitness{Version: acceptance.CloudV4PGWitnessVersion, Window: acceptance.CloudV4PGWindow{Attempts: 520, BaselineHash: p.PriorHistorySHA, PolicyHash: pin.PolicySHA256, Expires: a.Expires}}
	c := acceptance.CloudV4PGCorrelation{Status: "PG_WITNESS_CORRELATION_MATCH", ReportSHA256: p.ReportSHA, WitnessSHA256: p.PGSHA, TotalAttempts: 520, NewAttempts: 252, Rows: 252, Matched: true}
	for i := 0; i < 252; i++ {
		phase, role, caseID := "quality", acceptance.CloudRoleSingle, fmt.Sprintf("case:%032x", i)
		var parent *string
		if i >= 236 {
			phase = "comparison"
			role = []string{acceptance.CloudRoleSingle, acceptance.CloudRoleChildA, acceptance.CloudRoleChildB, acceptance.CloudRoleSynthesis}[(i-236)%4]
			caseID = fmt.Sprintf("case:%032x", (i-236)/4)
			if role == acceptance.CloudRoleChildA || role == acceptance.CloudRoleChildB {
				value := shadowV4ContextID(phase, caseID, acceptance.CloudRoleSynthesis)
				parent = &value
			}
		}
		context := shadowV4ContextID(phase, caseID, role)
		one, three := int64(1), int64(3)
		version, outputHash := p.ObservedVersion, strings.Repeat("8", 64)
		created := time.Now().Add(-time.Minute)
		sql := acceptance.CloudV4PGAttempt{Ordinal: 269 + i, Phase: phase, Role: role, CaseID: caseID, Context: context, ParentContextID: parent,
			InputHash: strings.Repeat("9", 64), Created: created, Outcome: "completed", InputTokens: &one, OutputTokens: &one, TotalTokens: &three, OutputHash: &outputHash}
		w.Attempts = append(w.Attempts, sql)
		a.Attempts = append(a.Attempts, acceptance.CloudV4SafeAttempt{CloudSafeAttempt: acceptance.CloudSafeAttempt{Ordinal: sql.Ordinal, Context: context, InputSHA256: sql.InputHash, Role: role, ChargedAt: created, Outcome: "completed", ModelVersion: &version, InputTokens: &one, OutputTokens: &one, TotalTokens: &three}, Phase: phase, ParentContextID: parent})
	}
	return p, history, a, w, c
}

func TestReleaseChainV4BridgeRequiresExactReviewedFullProof(t *testing.T) {
	p, h, a, w, c := syntheticReleaseChainV4Bridge(t)
	if validateReleaseChainV4Prerequisite(p, h, a, w, c) != nil {
		t.Fatal("synthetic internally consistent metadata denied")
	}
	for _, kind := range []string{"old_baseline", "failed_quality", "incomplete_window", "null_spend", "child_parent", "version_drift", "wrong_source_policy", "sql_mismatch", "wrong_report", "lower_generation"} {
		t.Run(kind, func(t *testing.T) {
			p, h, a, w, c := syntheticReleaseChainV4Bridge(t)
			switch kind {
			case "old_baseline":
				p.Baseline = 480
			case "failed_quality":
				a.Quality.Status = "NO_GO"
			case "incomplete_window":
				w.Window.Attempts = 519
			case "null_spend":
				w.Attempts[0].TotalTokens, a.Attempts[0].TotalTokens = nil, nil
			case "child_parent":
				value := "foreign-child-parent"
				w.Attempts[237].ParentContextID, a.Attempts[237].ParentContextID = &value, &value
			case "version_drift":
				value := "gemini-3.8-flash-other"
				a.Attempts[0].ModelVersion = &value
			case "wrong_source_policy":
				a.Pin.PolicySHA256, w.Window.PolicyHash = strings.Repeat("a", 64), strings.Repeat("a", 64)
			case "sql_mismatch":
				c.Matched = false
			case "wrong_report":
				c.ReportSHA256 = strings.Repeat("b", 64)
			case "lower_generation":
				p.GenerationTokens = 8192
			}
			if validateReleaseChainV4Prerequisite(p, h, a, w, c) == nil {
				t.Fatal("incomplete, stale or contradictory synthetic bridge accepted")
			}
		})
	}
	// Consistent metadata never substitutes for a frozen artifact identity.
	if _, err := readReleaseChainV4Prerequisite(); releaseChainV4ReviewedPrerequisiteSHA == "" && err == nil {
		t.Fatal("synthetic checker inputs became native admission")
	}
}
