package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const CloudAuditedWindow = "issue33-high-shadow-20261012-baseline45"
const CloudAuditedCode = "f3e0cbc11dedc2fb8e547e61f0153bf78cde6f98"
const CloudWindowReportLimit = 2 << 20

// Expected hashes come from the independent prepared release, not this report.
type CloudWindowExpectation struct {
	SourceSHA256   string
	HistorySHA256  string
	HardwareSHA256 string
}

type cloudWindowResources struct {
	Calls          int    `json:"maximum_calls_per_arm"`
	Input          int64  `json:"maximum_input_bytes"`
	Output         int64  `json:"maximum_output_tokens"`
	AttemptSeconds int    `json:"maximum_attempt_seconds"`
	ArmSeconds     int    `json:"maximum_arm_seconds"`
	Concurrency    int    `json:"concurrency"`
	Memory         int64  `json:"client_memory_limit_bytes"`
	Pricing        string `json:"pricing_basis"`
	Invoice        bool   `json:"invoice_measured"`
	RemoteRAM      bool   `json:"remote_ram_measured"`
}

func (v *cloudWindowResources) UnmarshalJSON(raw []byte) error {
	type plain cloudWindowResources
	return DecodeStrict(raw, (*plain)(v), "maximum_calls_per_arm", "maximum_input_bytes", "maximum_output_tokens", "maximum_attempt_seconds", "maximum_arm_seconds", "concurrency", "client_memory_limit_bytes", "pricing_basis", "invoice_measured", "remote_ram_measured")
}
func fixedCloudWindowResources() cloudWindowResources {
	return cloudWindowResources{4, 65536, 8192, 120, 480, 1, 1536 << 20, "subscription_no_overage", false, false}
}

type cloudHistoricalArtifact struct {
	SHA      string `json:"report_sha256"`
	Release  string `json:"release_ref"`
	Attempts int    `json:"new_attempts"`
}

func (v *cloudHistoricalArtifact) UnmarshalJSON(raw []byte) error {
	type plain cloudHistoricalArtifact
	return DecodeStrict(raw, (*plain)(v), "report_sha256", "release_ref", "new_attempts")
}

type cloudHistoricalUsage struct {
	Run         string `json:"run_id"`
	Reservation string `json:"reservation_id"`
	Input       *int64 `json:"actual_input_tokens"`
	Output      *int64 `json:"actual_output_tokens"`
}

func (v *cloudHistoricalUsage) UnmarshalJSON(raw []byte) error {
	type plain cloudHistoricalUsage
	return cloudDecode(raw, (*plain)(v), []string{"actual_input_tokens", "actual_output_tokens"}, "run_id", "reservation_id", "actual_input_tokens", "actual_output_tokens")
}

type cloudHistorical struct {
	Status    string                    `json:"status"`
	SHA       string                    `json:"baseline_sha256"`
	Artifacts []cloudHistoricalArtifact `json:"artifacts"`
	Attempts  int                       `json:"native_attempts"`
	Known     int                       `json:"known_usage_pairs"`
	Unknown   int                       `json:"unknown_usage_pairs"`
	Usage     []cloudHistoricalUsage    `json:"usage"`
}

func (v *cloudHistorical) UnmarshalJSON(raw []byte) error {
	type plain cloudHistorical
	return DecodeStrict(raw, (*plain)(v), "status", "baseline_sha256", "artifacts", "native_attempts", "known_usage_pairs", "unknown_usage_pairs", "usage")
}

type cloudChargedAttempt struct {
	Ordinal  int       `json:"attempt"`
	Context  string    `json:"run_id"`
	Kind     string    `json:"kind"`
	InputSHA string    `json:"input_hash"`
	Created  time.Time `json:"created_at"`
}

func (v *cloudChargedAttempt) UnmarshalJSON(raw []byte) error {
	type plain cloudChargedAttempt
	return DecodeStrict(raw, (*plain)(v), "attempt", "run_id", "kind", "input_hash", "created_at")
}

type cloudWindowEnvelope struct {
	Status           string                `json:"status"`
	Model            string                `json:"model"`
	Release          string                `json:"release_ref"`
	History          cloudHistorical       `json:"historical"`
	Total            int                   `json:"native_attempts"`
	New              int                   `json:"new_native_attempts"`
	Maximum          int                   `json:"maximum_total_attempts"`
	MaximumNew       int                   `json:"maximum_new_attempts"`
	Resources        cloudWindowResources  `json:"declared_resources"`
	Pin              CloudPin              `json:"pin"`
	Attempts         []CloudAttempt        `json:"quality_attempts"`
	StoredQuality    json.RawMessage       `json:"quality"`
	Single           CloudComparisonRun    `json:"single"`
	Multi            CloudComparisonRun    `json:"multi"`
	StoredComparison json.RawMessage       `json:"comparison"`
	Ledger           []cloudChargedAttempt `json:"charged_attempts"`
	Start            time.Time             `json:"started_at_utc"`
	End              time.Time             `json:"finished_at_utc"`
	Execution        bool                  `json:"execution_authorized"`
}

// This projection deliberately has no model output, case input or documents.
type CloudSafeAttempt struct {
	Ordinal      int       `json:"ordinal"`
	Context      string    `json:"context_id"`
	InputSHA256  string    `json:"input_sha256"`
	Role         string    `json:"role"`
	ChargedAt    time.Time `json:"charged_at_utc"`
	Start        time.Time `json:"start_utc"`
	End          time.Time `json:"end_utc"`
	Milliseconds int64     `json:"milliseconds"`
	Outcome      string    `json:"outcome"`
	ModelVersion *string   `json:"provider_model_version"`
	InputTokens  *int64    `json:"input_tokens"`
	OutputTokens *int64    `json:"output_tokens"`
	TotalTokens  *int64    `json:"total_tokens"`
	ClientRSS    *int64    `json:"client_process_peak_rss_bytes"`
}
type CloudWindowAudit struct {
	Status               string                  `json:"status"`
	EvidenceKind         string                  `json:"evidence_kind"`
	OriginalReportSHA256 string                  `json:"original_private_report_sha256"`
	CollectorStatus      string                  `json:"collector_status"`
	Pin                  CloudPin                `json:"pin"`
	HistorySHA256        string                  `json:"historical_baseline_sha256"`
	HistoricalAttempts   int                     `json:"historical_attempts"`
	HistoricalArtifacts  int                     `json:"historical_artifacts"`
	HistoricalKnown      int                     `json:"historical_known_pairs"`
	HistoricalUnknown    int                     `json:"historical_null_pairs"`
	HistoricalInput      int64                   `json:"historical_known_input_subtotal"`
	HistoricalOutput     int64                   `json:"historical_known_output_subtotal"`
	NewAttempts          int                     `json:"new_attempts"`
	CarryForwardAttempts int                     `json:"carry_forward_attempts"`
	MaximumNew           int                     `json:"maximum_new_attempts"`
	MaximumTotal         int                     `json:"maximum_total_attempts"`
	CorrelationValid     bool                    `json:"charged_ledger_correlated"`
	Attempts             []CloudSafeAttempt      `json:"attempts"`
	Cost                 CloudCost               `json:"all_new_attempt_cost"`
	Quality              CloudQualityReport      `json:"quality_recheck"`
	WitnessDiagnostics   CloudWitnessDiagnostics `json:"witness_diagnostics"`
	Comparison           CloudComparisonReport   `json:"comparison_recheck"`
	QualityValid         bool                    `json:"quality_recheck_valid"`
	ComparisonValid      bool                    `json:"comparison_recheck_valid"`
	ModelFamily          string                  `json:"observed_model_family_status"`
	UnknownVersions      int                     `json:"unknown_model_version_attempts"`
	Reasons              []string                `json:"no_go_reasons"`
	Execution            bool                    `json:"execution_authorized"`
	Limits               []string                `json:"limits"`
}

var cloudWindowContext = regexp.MustCompile(`^shadow-[a-f0-9]{32}$`)
var cloudWindowRuntime = regexp.MustCompile(`^go1\.27\.2/race; sha256:[a-f0-9]{64}$`)
var cloudSafeVersion = regexp.MustCompile(`^gemini-[a-zA-Z0-9][a-zA-Z0-9._-]{0,110}$`)
var cloudExpectedFamily = regexp.MustCompile(`^gemini-3\.8-flash([.-][a-zA-Z0-9._-]+)?$`)

func historyArtifactDigest(artifacts []cloudHistoricalArtifact) string {
	var first, high string
	for _, a := range artifacts[:4] {
		first += a.SHA + "\n"
	}
	high = hashWindowBytes([]byte(first)) + "\n"
	for _, a := range artifacts[4:9] {
		high += a.SHA + "\n"
	}
	baseline := hashWindowBytes([]byte(high))
	for _, a := range artifacts[9:] {
		baseline = hashWindowBytes([]byte(baseline + "\n" + a.SHA + "\n"))
	}
	return baseline
}
func hashWindowBytes(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

// AuditCloudWindow imports a collector claim offline. Independent signed gold
// and external release expectations are required; it cannot create a window,
// infer, sign approvals, mutate evidence, or authorize execution.
func AuditCloudWindow(c Corpus, approval GoldenApproval, key []byte, raw []byte, expected CloudWindowExpectation) (CloudWindowAudit, error) {
	r := CloudWindowAudit{Status: "NO_GO", EvidenceKind: "offline_window_audit", OriginalReportSHA256: hashWindowBytes(raw), Cost: newCloudCost(), Attempts: []CloudSafeAttempt{}, Reasons: []string{}, Limits: []string{
		"Independent role on a shared administrator-controlled host; not hardware isolation or blind evaluation.",
		"Collector provenance requires the independent immutable runner and owning PostgreSQL journal; this tool has no provider or database access.",
		"Requested High profile and observed Flash family do not prove proprietary weight bytes, server effort or remote hardware/memory.",
		"Every charged attempt remains spent, including failures and NULL usage; NO_GO never refunds or reopens a window.",
	}}
	deny := func() (CloudWindowAudit, error) { return r, errors.New("cloud window evidence denied") }
	if len(raw) == 0 || len(raw) > CloudWindowReportLimit || !validSHA(expected.SourceSHA256) || !validSHA(expected.HistorySHA256) || !validSHA(expected.HardwareSHA256) {
		return deny()
	}
	var e cloudWindowEnvelope
	if DecodeStrict(raw, &e, "status", "model", "release_ref", "historical", "native_attempts", "new_native_attempts", "maximum_total_attempts", "maximum_new_attempts", "declared_resources", "pin", "quality_attempts", "quality", "single", "multi", "comparison", "charged_attempts", "started_at_utc", "finished_at_utc", "execution_authorized") != nil {
		return deny()
	}
	if e.Status != "passed" && e.Status != "failed" || e.Model != CloudRequestedModel || e.Release != CloudAuditedCode || e.Pin.CodeCommit != CloudAuditedCode || e.Pin.SourceSHA256 != expected.SourceSHA256 || e.Pin.HardwareSHA256 != expected.HardwareSHA256 || e.Pin.ProviderModelVersion != nil || e.Pin.WindowID != CloudAuditedWindow || e.Pin.EvidenceKind != "native_cloud_observation" || !cloudWindowRuntime.MatchString(e.Pin.Runtime) || validateCloudPin(c, e.Pin) != nil || e.Resources != fixedCloudWindowResources() || e.Pin.ResourcesSHA256 != cloudDigest(e.Resources) || e.Execution || e.Start.IsZero() || e.End.Before(e.Start) || e.Maximum != 297 || e.MaximumNew != 252 || e.New < 0 || e.New > 252 || e.Total != 45+e.New {
		return deny()
	}
	if err := VerifyGolden(c, approval, key); err != nil {
		return deny()
	}
	h := e.History
	if h.Status != "mixed_completed_history" || h.SHA != expected.HistorySHA256 || h.Attempts != 45 || h.Known != 20 || h.Unknown != 25 || len(h.Usage) != 45 || len(h.Artifacts) != 15 {
		return deny()
	}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4}
	seenArtifacts := map[string]bool{}
	for i, a := range h.Artifacts {
		if !validSHA(a.SHA) || seenArtifacts[a.SHA] || a.Attempts != counts[i] || a.Release != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(a.Release) {
			return deny()
		}
		seenArtifacts[a.SHA] = true
	}
	if historyArtifactDigest(h.Artifacts) != h.SHA {
		return deny()
	}
	known, unknown := 0, 0
	for _, u := range h.Usage {
		if u.Input == nil && u.Output == nil {
			unknown++
			continue
		}
		if u.Input == nil || u.Output == nil || *u.Input < 0 || *u.Output < 0 || *u.Input > 74698-r.HistoricalInput || *u.Output > 2210-r.HistoricalOutput {
			return deny()
		}
		known++
		r.HistoricalInput += *u.Input
		r.HistoricalOutput += *u.Output
	}
	if known != 20 || unknown != 25 || r.HistoricalInput != 74698 || r.HistoricalOutput != 2210 {
		return deny()
	}
	r.Pin = e.Pin
	r.HistorySHA256 = h.SHA
	r.HistoricalAttempts = 45
	r.HistoricalArtifacts = 15
	r.HistoricalKnown = 20
	r.HistoricalUnknown = 25
	r.CollectorStatus = e.Status
	r.NewAttempts = e.New
	r.CarryForwardAttempts = e.Total
	r.MaximumNew = 252
	r.MaximumTotal = 297
	all := append([]CloudAttempt{}, e.Attempts...)
	for _, run := range []CloudComparisonRun{e.Single, e.Multi} {
		for _, task := range run.Tasks {
			all = append(all, task.Attempts...)
		}
	}
	if len(all) != e.New || len(e.Ledger) != e.New {
		return deny()
	}
	ledger := map[string]cloudChargedAttempt{}
	seenContexts, seenOrdinals := map[string]bool{}, map[int]bool{}
	for _, a := range e.Ledger {
		if a.Ordinal < 46 || a.Ordinal > e.Total || seenOrdinals[a.Ordinal] || seenContexts[a.Context] || !cloudWindowContext.MatchString(a.Context) || !validSHA(a.InputSHA) || a.Kind != "start" || a.Created.Before(e.Start) || a.Created.After(e.End) || a.Created.After(e.Start.Add(75*time.Minute)) {
			return deny()
		}
		seenOrdinals[a.Ordinal] = true
		seenContexts[a.Context] = true
		ledger[fmt.Sprintf("native-%03d", a.Ordinal)] = a
	}
	for i := 46; i <= e.Total; i++ {
		if !seenOrdinals[i] {
			return deny()
		}
	}
	seen := map[string]bool{}
	var observed *string
	wrongFamily := false
	for _, a := range all {
		l, ok := ledger[a.AttemptID]
		validRole := a.Role == CloudRoleSingle || a.Role == CloudRoleChildA || a.Role == CloudRoleChildB || a.Role == CloudRoleSynthesis
		if !ok || !validRole || seen[a.AttemptID] || l.Context != a.ContextID || l.InputSHA != a.InputSHA256 || validateCloudAttempt(a) != nil || a.StartUTC.Before(e.Start) || a.EndUTC.After(e.End) || l.Created.Before(a.StartUTC) || l.Created.After(a.EndUTC) {
			return deny()
		}
		seen[a.AttemptID] = true
		if a.ProviderModelVersion == nil {
			r.UnknownVersions++
		} else {
			if !cloudSafeVersion.MatchString(*a.ProviderModelVersion) {
				return deny()
			}
			wrongFamily = wrongFamily || !cloudExpectedFamily.MatchString(*a.ProviderModelVersion)
			if observed != nil && *observed != *a.ProviderModelVersion {
				r.Reasons = append(r.Reasons, "conflicting_observed_model_versions")
			}
			observed = a.ProviderModelVersion
		}
		if addCloudCost(&r.Cost, a) != nil {
			return deny()
		}
		r.Attempts = append(r.Attempts, CloudSafeAttempt{l.Ordinal, a.ContextID, a.InputSHA256, a.Role, l.Created, a.StartUTC, a.EndUTC, a.Milliseconds, a.Outcome, a.ProviderModelVersion, a.InputTokens, a.OutputTokens, a.TotalTokens, a.LocalPeakRSSBytes})
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
	}
	if wrongFamily {
		r.ModelFamily = "other_reported_family"
		r.Reasons = append(r.Reasons, "requested_3.8_flash_family_not_observed")
	}
	if e.Pin.ProviderModelVersion != nil && (!cloudSafeVersion.MatchString(*e.Pin.ProviderModelVersion) || !cloudExpectedFamily.MatchString(*e.Pin.ProviderModelVersion)) {
		return deny()
	}
	var err error
	r.Quality, err = ScoreCloud(c, approval, key, e.Pin, e.Attempts)
	r.WitnessDiagnostics = cloudWitnessDiagnostics(c, e.Attempts)
	r.QualityValid = err == nil
	if err != nil {
		r.Reasons = append(r.Reasons, "quality_recheck_denied")
	}
	r.Comparison, err = CompareCloud(c, approval, key, e.Single, e.Multi)
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
	if !r.QualityValid || r.Quality.Status != "CLOUD_SHADOW_THRESHOLDS_PASS" || !r.Quality.Complete || !r.Quality.ThresholdsPassed || !r.Quality.Cost.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "quality_or_full_cost_no_go")
	}
	if !r.ComparisonValid || r.Comparison.Status != "CLOUD_SHADOW_COMPARISON_PASS" || r.Comparison.PairedTasks != 4 || r.Comparison.SingleCorrect != 4 || r.Comparison.MultiCorrect != 4 || !r.Comparison.ThresholdsPassed || !r.Comparison.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "comparison_or_full_cost_no_go")
	}
	if e.New == 0 {
		r.Cost.FullTokenCostKnown = false
	}
	if !r.Cost.FullTokenCostKnown {
		r.Reasons = append(r.Reasons, "all_attempt_token_cost_unknown")
	}
	// Scorer reports contain fixed metrics only, never raw inputs/outputs. Always
	// retain the independently expected safe pin, even on invalid comparison data.
	r.Quality.Pin = e.Pin
	if len(r.Reasons) == 0 {
		r.Status = "CLOUD_WINDOW_AUDIT_PASS"
	}
	return r, nil
}

func sameCloudPin(a, b CloudPin) bool { return cloudDigest(a) == cloudDigest(b) }

// SafeFamilyVersion reports only bounded provider version metadata; it cannot
// map an opaque revision suffix to proprietary server effort or weight bytes.
func SafeCloudFamilyVersion(version string) bool {
	return cloudExpectedFamily.MatchString(strings.TrimSpace(version)) && cloudSafeVersion.MatchString(version)
}
