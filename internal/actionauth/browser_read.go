package actionauth

import (
	"encoding/json"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const BrowserGETVerification = "guarded_browser_get_v1"

func browserReadRejectionDigest(reason string) string {
	proof, _ := json.Marshal(map[string]any{"read_rejection": "pre_navigation_v1", "navigation_dispatched": false, "reason": reason})
	return digest(string(proof))
}

// PG accepts absence only for the owning read contract and a normalized digest
// of one of the closed pre-navigation proofs accepted by the service.
func browserReadAbsenceEvidence(b ActionBinding, p EffectPolicy, outcome string, e EffectEvidence) bool {
	if b.Effect != EffectRead || b.ContractRevision != "2" || (b.ToolName != "browser.submit_get_form" && b.ToolName != "browser.follow_link") || len(b.CredentialRefs) != 0 || p.RetrySafety != "read_only" || p.VerificationMethod != BrowserGETVerification || outcome != "dispatch_completed" || e.ArtifactDigest != "" {
		return false
	}
	for _, reason := range []string{"page_changed", "stale_observation", "get_form_denied", "invalid_args", "invalid_selector"} {
		if e.ResultDigest == browserReadRejectionDigest(reason) {
			return true
		}
	}
	return false
}

// BrowserReadRejection proves only that this exact GET operation never reached
// navigation. It is not an external recipient receipt or reusable permission.
// Revision1/opaque calls and a generic policy error cannot borrow this proof.
func BrowserReadRejection(b ActionBinding, result *toolbrokerv1.ToolResult) string {
	if b.Effect != EffectRead || b.ContractRevision != "2" || (b.ToolName != "browser.submit_get_form" && b.ToolName != "browser.follow_link") || len(b.CredentialRefs) != 0 || result == nil || result.GetStatus() != "failed" || result.GetRunId() != b.RunID || result.GetToolCallId() != b.ToolCallID || result.GetToolName() != b.ToolName || (result.GetResultJson() != "" && result.GetResultJson() != "{}") {
		return ""
	}
	failure := result.GetError()
	if failure == nil || failure.GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED && failure.GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR || len(failure.GetDetailsJson()) > 1024 {
		return ""
	}
	raw := failure.GetDetailsJson()
	if _, err := canonicalJSON(raw); err != nil {
		return ""
	}
	var proof struct {
		Protocol   string `json:"read_rejection"`
		Navigation *bool  `json:"navigation_dispatched"`
		Reason     string `json:"reason"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil || len(fields) != 3 || json.Unmarshal([]byte(raw), &proof) != nil || proof.Protocol != "pre_navigation_v1" || proof.Navigation == nil || *proof.Navigation {
		return ""
	}
	for _, key := range []string{"read_rejection", "navigation_dispatched", "reason"} {
		if _, exists := fields[key]; !exists {
			return ""
		}
	}
	switch proof.Reason {
	case "page_changed", "stale_observation", "get_form_denied", "invalid_args", "invalid_selector":
		return proof.Reason
	}
	return ""
}
