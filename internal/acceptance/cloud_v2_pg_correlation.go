package acceptance

import (
	"errors"
	"time"
)

const CloudV2PGWitnessVersion = "butler-cloud-v2-pg-witness-v1"

type CloudV2PGWindow struct {
	Attempts     int       `json:"attempts"`
	BaselineHash string    `json:"baseline_hash"`
	PolicyHash   string    `json:"policy_hash"`
	Expires      time.Time `json:"expires_at"`
}

func (w *CloudV2PGWindow) UnmarshalJSON(raw []byte) error {
	type plain CloudV2PGWindow
	return DecodeStrict(raw, (*plain)(w), "attempts", "baseline_hash", "policy_hash", "expires_at")
}

type CloudV2PGAttempt struct {
	Ordinal      int       `json:"ordinal"`
	Phase        string    `json:"phase"`
	CaseID       string    `json:"case_id"`
	Role         string    `json:"role"`
	Context      string    `json:"context_id"`
	InputHash    string    `json:"input_hash"`
	Created      time.Time `json:"created_at"`
	Outcome      string    `json:"outcome"`
	InputTokens  *int64    `json:"input_tokens"`
	OutputTokens *int64    `json:"output_tokens"`
	TotalTokens  *int64    `json:"total_tokens"`
	OutputHash   *string   `json:"output_hash"`
}

func (a *CloudV2PGAttempt) UnmarshalJSON(raw []byte) error {
	type plain CloudV2PGAttempt
	return cloudDecode(raw, (*plain)(a), []string{"input_tokens", "output_tokens", "total_tokens", "output_hash"}, "ordinal", "phase", "case_id", "role", "context_id", "input_hash", "created_at", "outcome", "input_tokens", "output_tokens", "total_tokens", "output_hash")
}

type CloudV2PGWitness struct {
	Version  string             `json:"version"`
	Window   CloudV2PGWindow    `json:"window"`
	Attempts []CloudV2PGAttempt `json:"attempts"`
}

type CloudV2PGCorrelation struct {
	Status          string `json:"status"`
	ReportSHA256    string `json:"collector_report_sha256"`
	WitnessSHA256   string `json:"pg_witness_sha256"`
	TotalAttempts   int    `json:"carry_forward_attempts"`
	NewAttempts     int    `json:"new_attempts"`
	Rows            int    `json:"witness_rows"`
	Matched         bool   `json:"exact_journal_match"`
	QualityPassed   bool   `json:"quality_passed"`
	InferenceProven bool   `json:"inference_proven"`
	Execution       bool   `json:"execution_authorized"`
}

// CorrelateCloudV2PGWitness compares independent exported SQL metadata with the
// collector's projection. It has no provider, storage, executor, signing or
// quality capability. The owning immutable runner/source must establish export
// provenance separately; equality alone cannot establish native inference.
func CorrelateCloudV2PGWitness(reportRaw, witnessRaw []byte) (CloudV2PGCorrelation, error) {
	r := CloudV2PGCorrelation{Status: "NO_GO", ReportSHA256: hashWindowBytes(reportRaw), WitnessSHA256: hashWindowBytes(witnessRaw)}
	deny := func() (CloudV2PGCorrelation, error) {
		return r, errors.New("prospective PG witness correlation denied")
	}
	if len(reportRaw) == 0 || len(witnessRaw) == 0 || len(reportRaw) > CloudWindowReportLimit || len(witnessRaw) > CloudWindowReportLimit {
		return deny()
	}
	var w CloudV2PGWitness
	if DecodeStrict(witnessRaw, &w, "version", "window", "attempts") != nil || w.Version != CloudV2PGWitnessVersion || w.Window.Attempts < 228 || w.Window.Attempts > 480 || !validSHA(w.Window.BaselineHash) || !validSHA(w.Window.PolicyHash) || w.Window.Expires.IsZero() || len(w.Attempts) != w.Window.Attempts-228 {
		return deny()
	}
	r.TotalAttempts, r.NewAttempts, r.Rows = w.Window.Attempts, w.Window.Attempts-228, len(w.Attempts)
	contexts, selections := map[string]bool{}, map[string]bool{}
	for i, a := range w.Attempts {
		role := a.Role == CloudRoleSingle || a.Role == CloudRoleChildA || a.Role == CloudRoleChildB || a.Role == CloudRoleSynthesis
		selection := a.Phase + "\x00" + a.CaseID + "\x00" + a.Role
		if a.Ordinal != 229+i || !role || a.Phase != "quality" && a.Phase != "comparison" || a.Phase == "quality" && a.Role != CloudRoleSingle || a.CaseID == "" || !cloudWindowContext.MatchString(a.Context) || contexts[a.Context] || selections[selection] || !validSHA(a.InputHash) || a.Created.IsZero() || !a.Created.Before(w.Window.Expires) {
			return deny()
		}
		contexts[a.Context], selections[selection] = true, true
		if i > 0 && a.Created.Before(w.Attempts[i-1].Created) {
			return deny()
		}
		switch a.Outcome {
		case "unavailable", "completed", "transport_error", "cancelled", "timeout", "invalid_tool_proposal":
		default:
			return deny()
		}
		for _, value := range []*int64{a.InputTokens, a.OutputTokens, a.TotalTokens} {
			if value != nil && *value < 0 {
				return deny()
			}
		}
		if a.Outcome == "completed" {
			if a.OutputHash == nil || !validSHA(*a.OutputHash) {
				return deny()
			}
		} else if a.OutputHash != nil {
			return deny()
		}
	}
	var e cloudV2WindowEnvelope
	if DecodeStrict(reportRaw, &e, "status", "model", "release_ref", "historical", "native_attempts", "new_native_attempts", "maximum_total_attempts", "maximum_new_attempts", "declared_resources", "pin", "quality_attempts", "quality", "single", "multi", "comparison", "charged_attempts", "started_at_utc", "finished_at_utc", "window_expires_at_utc", "execution_authorized") != nil {
		return deny()
	}
	// Mismatches are content-free results, retaining the independent SQL count.
	// Failed collectors can match their SQL journal and still remain NO_GO under
	// the separate window audit, which owns costs and quality readiness.
	if e.Total != w.Window.Attempts || e.New != r.NewAttempts || e.Maximum != 480 || e.MaximumNew != 252 || e.History.SHA != w.Window.BaselineHash || e.Pin.PolicySHA256 != w.Window.PolicyHash || !e.Expires.Equal(w.Window.Expires) || len(e.Ledger) != len(w.Attempts) {
		return r, nil
	}
	for i, sql := range w.Attempts {
		collector := e.Ledger[i]
		if collector.Ordinal != sql.Ordinal || collector.Kind != "start" || collector.Phase != sql.Phase || collector.CaseID != sql.CaseID || collector.Role != sql.Role || collector.Context != sql.Context || collector.InputSHA != sql.InputHash || !collector.Created.Equal(sql.Created) || collector.Outcome != sql.Outcome || !equalCloudNullable(collector.InputTokens, sql.InputTokens) || !equalCloudNullable(collector.OutputTokens, sql.OutputTokens) || !equalCloudNullable(collector.TotalTokens, sql.TotalTokens) || !equalCloudV2NullableString(collector.OutputSHA, sql.OutputHash) {
			return r, nil
		}
	}
	r.Matched, r.Status = true, "PG_WITNESS_CORRELATION_MATCH"
	return r, nil
}

func equalCloudV2NullableString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
