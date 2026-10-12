package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

const CloudV2AuditedWindow = "issue33-high-shadow-v2-20261012-baseline228"
const CloudV2PreviousPrivateSHA256 = "b82c572c62840e9e18de9723fc33bf3d335f546e4d445a4bc0a16a1a81686b99"
const CloudV2PreviousAuditSHA256 = "332d10777cb208b4d78fdfbf88be5183050ab4679ba3cf372395bdab74eab249"

type CloudV2WindowExpectation struct {
	CodeCommit, SourceSHA256, HardwareSHA256, HistorySHA256 string
}

func (v *CloudV2Resources) UnmarshalJSON(raw []byte) error {
	type plain CloudV2Resources
	return DecodeStrict(raw, (*plain)(v), "maximum_calls_per_arm", "maximum_input_bytes", "maximum_output_tokens", "maximum_visible_json_bytes", "maximum_attempt_seconds", "maximum_arm_seconds", "concurrency", "client_memory_limit_bytes", "pricing_basis", "invoice_measured", "remote_ram_measured")
}

type cloudV2ChargedAttempt struct {
	Ordinal      int       `json:"attempt"`
	Context      string    `json:"run_id"`
	Kind         string    `json:"kind"`
	InputSHA     string    `json:"input_hash"`
	Created      time.Time `json:"created_at"`
	Phase        string    `json:"phase"`
	CaseID       string    `json:"case_id"`
	Role         string    `json:"role"`
	Outcome      string    `json:"outcome"`
	InputTokens  *int64    `json:"input_tokens"`
	OutputTokens *int64    `json:"output_tokens"`
	TotalTokens  *int64    `json:"total_tokens"`
	OutputSHA    *string   `json:"output_sha256"`
}

func (v *cloudV2ChargedAttempt) UnmarshalJSON(raw []byte) error {
	type plain cloudV2ChargedAttempt
	return cloudDecode(raw, (*plain)(v), []string{"input_tokens", "output_tokens", "total_tokens", "output_sha256"}, "attempt", "run_id", "kind", "input_hash", "created_at", "phase", "case_id", "role", "outcome", "input_tokens", "output_tokens", "total_tokens", "output_sha256")
}

type cloudV2WindowEnvelope struct {
	Status           string                  `json:"status"`
	Model            string                  `json:"model"`
	Release          string                  `json:"release_ref"`
	History          cloudHistorical         `json:"historical"`
	Total            int                     `json:"native_attempts"`
	New              int                     `json:"new_native_attempts"`
	Maximum          int                     `json:"maximum_total_attempts"`
	MaximumNew       int                     `json:"maximum_new_attempts"`
	Resources        CloudV2Resources        `json:"declared_resources"`
	Pin              CloudPin                `json:"pin"`
	Attempts         []CloudAttempt          `json:"quality_attempts"`
	StoredQuality    json.RawMessage         `json:"quality"`
	Single           CloudComparisonRun      `json:"single"`
	Multi            CloudComparisonRun      `json:"multi"`
	StoredComparison json.RawMessage         `json:"comparison"`
	Ledger           []cloudV2ChargedAttempt `json:"charged_attempts"`
	Start            time.Time               `json:"started_at_utc"`
	End              time.Time               `json:"finished_at_utc"`
	Expires          time.Time               `json:"window_expires_at_utc"`
	Execution        bool                    `json:"execution_authorized"`
}

type CloudV2SafeAttempt struct {
	CloudSafeAttempt
	Phase string `json:"phase"`
}

// This content-free projection never includes questions, documents, model
// outputs, golden labels, keys, signatures or diagnostic narrative.
type CloudV2WindowAudit struct {
	Status               string                `json:"status"`
	EvidenceKind         string                `json:"evidence_kind"`
	OriginalReportSHA256 string                `json:"original_private_report_sha256"`
	CollectorStatus      string                `json:"collector_status"`
	Pin                  CloudPin              `json:"pin"`
	HistorySHA256        string                `json:"historical_baseline_sha256"`
	PreviousPrivateSHA   string                `json:"previous_failed_private_sha256"`
	PreviousAuditSHA     string                `json:"previous_failed_audit_sha256"`
	HistoricalAttempts   int                   `json:"historical_attempts"`
	HistoricalArtifacts  int                   `json:"historical_artifacts"`
	HistoricalKnown      int                   `json:"historical_known_pairs"`
	HistoricalUnknown    int                   `json:"historical_null_pairs"`
	HistoricalInput      int64                 `json:"historical_known_input_subtotal"`
	HistoricalOutput     int64                 `json:"historical_known_output_subtotal"`
	HistoricalTotal      int64                 `json:"historical_known_total_subtotal"`
	HistoricalNullTotal  int                   `json:"historical_unknown_total_attempts"`
	NewAttempts          int                   `json:"new_attempts"`
	CarryForwardAttempts int                   `json:"carry_forward_attempts"`
	MaximumNew           int                   `json:"maximum_new_attempts"`
	MaximumTotal         int                   `json:"maximum_total_attempts"`
	Start                time.Time             `json:"started_at_utc"`
	End                  time.Time             `json:"finished_at_utc"`
	Expires              time.Time             `json:"window_expires_at_utc"`
	CorrelationValid     bool                  `json:"charged_ledger_correlated"`
	Attempts             []CloudV2SafeAttempt  `json:"attempts"`
	Cost                 CloudCost             `json:"all_new_attempt_cost"`
	Quality              CloudV2QualityReport  `json:"quality_recheck"`
	Comparison           CloudComparisonReport `json:"comparison_recheck"`
	QualityValid         bool                  `json:"quality_recheck_valid"`
	ComparisonValid      bool                  `json:"comparison_recheck_valid"`
	ModelFamily          string                `json:"observed_model_family_status"`
	UnknownVersions      int                   `json:"unknown_model_version_attempts"`
	Reasons              []string              `json:"no_go_reasons"`
	Execution            bool                  `json:"execution_authorized"`
	Limits               []string              `json:"limits"`
}

// AuditCloudV2Window is an offline import. Trusted expectations come from the
// independently prepared source/hardware/history, never from the report itself.
// A scorer error still returns all correlated spend and a safe NO_GO result.
func AuditCloudV2Window(c CloudV2Corpus, approval GoldenApproval, key []byte, raw []byte, expected CloudV2WindowExpectation) (CloudV2WindowAudit, error) {
	r := CloudV2WindowAudit{Status: "NO_GO", EvidenceKind: "offline_v2_window_audit", OriginalReportSHA256: hashWindowBytes(raw), Cost: newCloudCost(), Attempts: []CloudV2SafeAttempt{}, Reasons: []string{}, Limits: []string{
		"Independent review role on a shared administrator-controlled host, not hardware isolation or blind evaluation.",
		"A signed golden and ledger correlation do not authorize execution; collector provenance also requires the immutable runner and owning PostgreSQL journal.",
		"Requested High plus an observed 3.8 Flash family does not prove cloud weight bytes, server effort or remote hardware/RAM.",
		"All charged failures and NULL usage remain spent; no audit verdict can refund or reopen an old window.",
		"Historical totals for the first 45 attempts are unavailable. The known historical total subtotal covers only the pinned 183-call failed observation.",
	}}
	deny := func() (CloudV2WindowAudit, error) { return r, errors.New("prospective window evidence denied") }
	if len(raw) == 0 || len(raw) > CloudWindowReportLimit || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(expected.CodeCommit) || !validSHA(expected.SourceSHA256) || !validSHA(expected.HistorySHA256) || !validSHA(expected.HardwareSHA256) {
		return deny()
	}
	var e cloudV2WindowEnvelope
	if DecodeStrict(raw, &e, "status", "model", "release_ref", "historical", "native_attempts", "new_native_attempts", "maximum_total_attempts", "maximum_new_attempts", "declared_resources", "pin", "quality_attempts", "quality", "single", "multi", "comparison", "charged_attempts", "started_at_utc", "finished_at_utc", "window_expires_at_utc", "execution_authorized") != nil {
		return deny()
	}
	if e.Status != "passed" && e.Status != "failed" || e.Model != CloudRequestedModel || e.Release != expected.CodeCommit || e.Pin.CodeCommit != expected.CodeCommit || e.Pin.SourceSHA256 != expected.SourceSHA256 || e.Pin.HardwareSHA256 != expected.HardwareSHA256 || e.Pin.ProviderModelVersion != nil || e.Pin.WindowID != CloudV2AuditedWindow || e.Pin.EvidenceKind != "native_cloud_observation" || !cloudWindowRuntime.MatchString(e.Pin.Runtime) || validateCloudV2Pin(c, e.Pin) != nil || e.Resources != FixedCloudV2Resources() || e.Execution || e.Start.IsZero() || e.End.Before(e.Start) || e.Expires.IsZero() || !e.Expires.After(e.Start) || e.Expires.After(e.Start.Add(75*time.Minute)) || e.Maximum != 480 || e.MaximumNew != 252 || e.New < 0 || e.New > 252 || e.Total != 228+e.New || VerifyCloudV2Golden(c, approval, key) != nil {
		return deny()
	}
	h := e.History
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4, 183}
	if h.Status != "mixed_completed_history" || h.SHA != expected.HistorySHA256 || h.Attempts != 228 || h.Known != 203 || h.Unknown != 25 || len(h.Usage) != 228 || len(h.Artifacts) != 16 {
		return deny()
	}
	seenArtifacts := map[string]bool{}
	for i, a := range h.Artifacts {
		if !validSHA(a.SHA) || seenArtifacts[a.SHA] || a.Attempts != counts[i] || a.Release != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(a.Release) {
			return deny()
		}
		seenArtifacts[a.SHA] = true
	}
	previous := h.Artifacts[15]
	baseline := historyArtifactDigest(h.Artifacts[:15])
	if previous.SHA != CloudV2PreviousAuditSHA256 || previous.Release != CloudAuditedCode || hashWindowBytes([]byte(baseline+"\n"+CloudV2PreviousPrivateSHA256+"\n"+CloudV2PreviousAuditSHA256+"\n")) != h.SHA {
		return deny()
	}
	known, unknown, oldKnown, oldUnknown := 0, 0, 0, 0
	var oldInput, oldOutput int64
	oldContexts := map[string]bool{}
	for i, u := range h.Usage {
		if u.Input == nil && u.Output == nil {
			if i >= 45 {
				return deny()
			}
			unknown++
			oldUnknown++
			continue
		}
		if u.Input == nil || u.Output == nil || *u.Input < 0 || *u.Output < 0 || *u.Input > 137405-r.HistoricalInput || *u.Output > 5293-r.HistoricalOutput {
			return deny()
		}
		known++
		r.HistoricalInput += *u.Input
		r.HistoricalOutput += *u.Output
		if i < 45 {
			oldKnown++
			oldInput += *u.Input
			oldOutput += *u.Output
		} else {
			if !cloudWindowContext.MatchString(u.Run) || oldContexts[u.Run] || u.Reservation != fmt.Sprintf("shadow-v1-native-%d", i+1) {
				return deny()
			}
			oldContexts[u.Run] = true
		}
	}
	if known != 203 || unknown != 25 || oldKnown != 20 || oldUnknown != 25 || oldInput != 74698 || oldOutput != 2210 || r.HistoricalInput != 137405 || r.HistoricalOutput != 5293 {
		return deny()
	}
	r.Pin, r.HistorySHA256, r.CollectorStatus = e.Pin, h.SHA, e.Status
	r.PreviousPrivateSHA, r.PreviousAuditSHA = CloudV2PreviousPrivateSHA256, CloudV2PreviousAuditSHA256
	r.HistoricalAttempts, r.HistoricalArtifacts, r.HistoricalKnown, r.HistoricalUnknown = 228, 16, 203, 25
	r.HistoricalTotal, r.HistoricalNullTotal = 218644, 45
	r.NewAttempts, r.CarryForwardAttempts, r.MaximumNew, r.MaximumTotal = e.New, e.Total, 252, 480
	r.Start, r.End, r.Expires = e.Start, e.End, e.Expires
	all := append([]CloudAttempt{}, e.Attempts...)
	phaseByAttempt := map[string]string{}
	for _, a := range e.Attempts {
		phaseByAttempt[a.AttemptID] = "quality"
	}
	for _, run := range []CloudComparisonRun{e.Single, e.Multi} {
		for _, task := range run.Tasks {
			all = append(all, task.Attempts...)
			for _, a := range task.Attempts {
				phaseByAttempt[a.AttemptID] = "comparison"
			}
		}
	}
	// Aggregate recorded measurements before scoring or semantic correlation.
	for _, a := range all {
		if validateCloudAttempt(a) != nil || addCloudCost(&r.Cost, a) != nil {
			return deny()
		}
	}
	if len(all) != e.New || len(e.Ledger) != e.New {
		return deny()
	}
	ledger := map[string]cloudV2ChargedAttempt{}
	ordinals, contexts, selections := map[int]bool{}, map[string]bool{}, map[string]bool{}
	// Exact once-only release order: all eligible originals, then the four fixed
	// single/child_a/child_b/synthesis pairs. Failed prefixes cannot resample.
	type plannedCall struct{ phase, caseID, role string }
	plan := []plannedCall{}
	inputs, _ := ExportCloudV2Inputs(c, "")
	pairs, _ := CloudV2ComparisonInputs(c)
	for _, input := range inputs.Cases {
		plan = append(plan, plannedCall{"quality", input.ID, CloudRoleSingle})
	}
	for _, input := range pairs {
		for _, role := range []string{CloudRoleSingle, CloudRoleChildA, CloudRoleChildB, CloudRoleSynthesis} {
			plan = append(plan, plannedCall{"comparison", input.ID, role})
		}
	}
	for _, l := range e.Ledger {
		context := "shadow-" + cloudDigest([]string{CloudV2AuditedWindow, l.Phase, l.CaseID, l.Role})[:32]
		selection := l.Phase + "\x00" + l.CaseID + "\x00" + l.Role
		if l.Ordinal < 229 || l.Ordinal > e.Total || ordinals[l.Ordinal] || contexts[l.Context] || selections[selection] || l.Context != context || !validSHA(l.InputSHA) || l.Kind != "start" || l.Phase != "quality" && l.Phase != "comparison" || l.Phase == "quality" && l.Role != CloudRoleSingle || l.Created.Before(e.Start) || l.Created.After(e.End) || !l.Created.Before(e.Expires) {
			return deny()
		}
		if want := plan[l.Ordinal-229]; l.Phase != want.phase || l.CaseID != want.caseID || l.Role != want.role {
			return deny()
		}
		ordinals[l.Ordinal], contexts[l.Context], selections[selection] = true, true, true
		ledger[fmt.Sprintf("native-%03d", l.Ordinal)] = l
	}
	for i := 229; i <= e.Total; i++ {
		if !ordinals[i] {
			return deny()
		}
	}
	seen := map[string]bool{}
	var observed *string
	for _, a := range all {
		l, ok := ledger[a.AttemptID]
		validRole := a.Role == CloudRoleSingle || a.Role == CloudRoleChildA || a.Role == CloudRoleChildB || a.Role == CloudRoleSynthesis
		if !ok || !validRole || seen[a.AttemptID] || l.Phase != phaseByAttempt[a.AttemptID] || l.Context != a.ContextID || l.InputSHA != a.InputSHA256 || l.CaseID != a.CaseID || l.Role != a.Role || l.Outcome != a.Outcome || !equalCloudNullable(l.InputTokens, a.InputTokens) || !equalCloudNullable(l.OutputTokens, a.OutputTokens) || !equalCloudNullable(l.TotalTokens, a.TotalTokens) || a.StartUTC.Before(e.Start) || a.EndUTC.After(e.End) || l.Created.Before(a.StartUTC) || l.Created.After(a.EndUTC) {
			return deny()
		}
		seen[a.AttemptID] = true
		if a.Outcome == "completed" {
			hash, err := CloudV2OutputDigest(a.Output)
			if err != nil || l.OutputSHA == nil || *l.OutputSHA != hash {
				return deny()
			}
		} else if l.OutputSHA != nil {
			return deny()
		}
		if a.ProviderModelVersion == nil {
			r.UnknownVersions++
		} else {
			if !cloudSafeVersion.MatchString(*a.ProviderModelVersion) {
				return deny()
			}
			if !SafeCloudFamilyVersion(*a.ProviderModelVersion) {
				r.Reasons = append(r.Reasons, "other_observed_model_family")
			}
			if observed != nil && *observed != *a.ProviderModelVersion {
				r.Reasons = append(r.Reasons, "conflicting_observed_model_versions")
			}
			observed = a.ProviderModelVersion
		}
		r.Attempts = append(r.Attempts, CloudV2SafeAttempt{CloudSafeAttempt{l.Ordinal, a.ContextID, a.InputSHA256, a.Role, l.Created, a.StartUTC, a.EndUTC, a.Milliseconds, a.Outcome, a.ProviderModelVersion, a.InputTokens, a.OutputTokens, a.TotalTokens, a.LocalPeakRSSBytes}, l.Phase})
	}
	sort.Slice(r.Attempts, func(i, j int) bool { return r.Attempts[i].Ordinal < r.Attempts[j].Ordinal })
	for i := 1; i < len(r.Attempts); i++ {
		if r.Attempts[i].Start.Before(r.Attempts[i-1].End) || r.Attempts[i].ChargedAt.Before(r.Attempts[i-1].ChargedAt) {
			return deny()
		}
	}
	r.CorrelationValid = true
	r.ModelFamily = "unavailable"
	if observed != nil {
		r.ModelFamily = "observed_3.8_flash_family"
		if !SafeCloudFamilyVersion(*observed) {
			r.ModelFamily = "other_reported_family"
		}
	}
	if r.UnknownVersions > 0 {
		r.Reasons = append(r.Reasons, "model_version_unavailable")
	}
	var err error
	r.Quality, err = ScoreCloudV2(c, approval, key, e.Pin, e.Attempts)
	r.QualityValid = err == nil
	if err != nil {
		r.Reasons = append(r.Reasons, "quality_recheck_denied")
	}
	r.Comparison, err = CompareCloudV2(c, approval, key, e.Single, e.Multi)
	r.ComparisonValid = err == nil
	if err != nil {
		r.Reasons = append(r.Reasons, "comparison_recheck_denied")
	}
	if !sameCloudPin(e.Pin, e.Single.Pin) || !sameCloudPin(e.Pin, e.Multi.Pin) {
		r.Reasons = append(r.Reasons, "comparison_pin_differs_from_window")
	}
	if e.Status != "passed" {
		r.Reasons = append(r.Reasons, "collector_failed")
	}
	if e.New != 252 {
		r.Reasons = append(r.Reasons, "incomplete_window")
	}
	if !r.QualityValid || r.Quality.Status != "CLOUD_V2_SHADOW_THRESHOLDS_PASS" || !r.Quality.Complete || !r.Quality.ThresholdsPassed || !r.Quality.Cost.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "quality_or_full_cost_no_go")
	}
	if !r.ComparisonValid || r.Comparison.Status != "CLOUD_V2_SHADOW_COMPARISON_PASS" || r.Comparison.PairedTasks != 4 || r.Comparison.SingleCorrect != 4 || r.Comparison.MultiCorrect != 4 || !r.Comparison.ThresholdsPassed || !r.Comparison.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "comparison_or_full_cost_no_go")
	}
	if e.New == 0 {
		r.Cost.FullTokenCostKnown = false
	}
	if !r.Cost.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "all_attempt_token_cost_unknown")
	}
	r.Quality.Pin = e.Pin
	if len(r.Reasons) == 0 {
		r.Status = "CLOUD_V2_WINDOW_AUDIT_PASS"
	}
	return r, nil
}

func equalCloudNullable(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
