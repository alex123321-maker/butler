package actionauth

import (
	"encoding/json"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func preparationReason(reason string) bool {
	switch reason {
	case "page_changed", "stale_observation", "invalid_args", "invalid_selector":
		return true
	}
	return false
}
func preparationRejectionDigest(reason string) string {
	raw, _ := json.Marshal(map[string]any{"preparation_rejection": "pre_input_v1", "input_dispatched": false, "reason": reason})
	return digest(string(raw))
}

// Revision3 adds only a closed proof that THIS local preparation never reached
// input dispatch. Revision2, opaque gestures, generic failures and lost ACKs
// retain their old uncertainty semantics. This is not a recipient receipt.
func BrowserPreparationRejection(b ActionBinding, r *toolbrokerv1.ToolResult) string {
	if b.Effect != EffectPrepare || (b.ContractRevision != "3" && b.ContractRevision != "4") || (b.ToolName != "browser.prepare_fill" && b.ToolName != "browser.prepare_type") || r == nil || r.GetStatus() != "failed" || r.GetRunId() != b.RunID || r.GetToolCallId() != b.ToolCallID || r.GetToolName() != b.ToolName || (r.GetResultJson() != "" && r.GetResultJson() != "{}") {
		return ""
	}
	e := r.GetError()
	if e == nil || (e.GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED && e.GetErrorClass() != commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR) || len(e.GetDetailsJson()) > 1024 {
		return ""
	}
	raw := e.GetDetailsJson()
	if _, err := canonicalJSON(raw); err != nil {
		return ""
	}
	var fields map[string]json.RawMessage
	var proof struct {
		Protocol   string `json:"preparation_rejection"`
		Dispatched *bool  `json:"input_dispatched"`
		Reason     string `json:"reason"`
	}
	if json.Unmarshal([]byte(raw), &fields) != nil || len(fields) != 3 || json.Unmarshal([]byte(raw), &proof) != nil || proof.Protocol != "pre_input_v1" || proof.Dispatched == nil || *proof.Dispatched || !(preparationReason(proof.Reason) || b.ContractRevision == "4" && proof.Reason == "session_lost") {
		return ""
	}
	for _, key := range []string{"preparation_rejection", "input_dispatched", "reason"} {
		if _, ok := fields[key]; !ok {
			return ""
		}
	}
	return proof.Reason
}

func preparationAbsenceEvidence(b ActionBinding, p EffectPolicy, outcome string, e EffectEvidence) bool {
	pinned := p.LocalPreparation
	if b.Effect != EffectPrepare || (b.ContractRevision != "3" && b.ContractRevision != "4") || (b.ToolName != "browser.prepare_fill" && b.ToolName != "browser.prepare_type") || pinned == nil || pinned.SessionID == "" || pinned.ObservationVersion == 0 || b.Resource != "browser:"+pinned.SessionID+":"+pinned.ExpectedURL || p.RetrySafety != "unsafe" || p.VerificationMethod != "local_browser_preparation_v1" || p.IdempotencyHeader != "" || p.ScopeOrigin != "" || p.VerificationURL != "" || outcome != "dispatch_completed" || e.ArtifactDigest != "" {
		return false
	}
	for _, reason := range []string{"page_changed", "stale_observation", "invalid_args", "invalid_selector"} {
		if e.ResultDigest == preparationRejectionDigest(reason) {
			return true
		}
	}
	if b.ContractRevision == "4" && e.ResultDigest == preparationRejectionDigest("session_lost") {
		return true
	}
	return false
}
