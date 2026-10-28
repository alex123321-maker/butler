package actionauth

import (
	"context"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const noNavigationProof = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":false,"reason":"page_changed"}`

func TestGuardedBrowserReadRejectionPinsPreNavigationOnly(t *testing.T) {
	for _, mode := range []string{"valid", "old_revision", "other_tool", "write", "foreign_call", "foreign_run", "credential", "missing_marker", "navigation_started", "null_navigation", "timeout", "extra_field", "duplicate_field", "payload", "internal_error"} {
		t.Run(mode, func(t *testing.T) {
			b := ActionBinding{RunID: "run", ToolCallID: "call", ToolName: "browser.submit_get_form", Effect: EffectRead, ContractRevision: "2"}
			r := &toolbrokerv1.ToolResult{RunId: "run", ToolCallId: "call", ToolName: b.ToolName, Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, DetailsJson: noNavigationProof}}
			switch mode {
			case "old_revision":
				b.ContractRevision = "1"
			case "other_tool":
				b.ToolName = "browser.click"
				r.ToolName = b.ToolName
			case "write":
				b.Effect = EffectCommit
			case "foreign_call":
				r.ToolCallId = "foreign"
			case "foreign_run":
				r.RunId = "foreign"
			case "credential":
				b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
			case "missing_marker":
				r.Error.DetailsJson = `{"reason":"page_changed"}`
			case "navigation_started":
				r.Error.DetailsJson = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":true,"reason":"page_changed"}`
			case "null_navigation":
				r.Error.DetailsJson = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":null,"reason":"page_changed"}`
			case "timeout":
				r.Error.DetailsJson = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":false,"reason":"timeout"}`
			case "extra_field":
				r.Error.DetailsJson = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":false,"reason":"page_changed","private":"data"}`
			case "duplicate_field":
				r.Error.DetailsJson = `{"read_rejection":"pre_navigation_v1","navigation_dispatched":true,"navigation_dispatched":false,"reason":"page_changed"}`
			case "payload":
				r.ResultJson = `{"ok":true}`
			case "internal_error":
				r.Error.ErrorClass = commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
			}
			if got := BrowserReadRejection(b, r); (got != "") != (mode == "valid") {
				t.Fatal("unproven non-navigation accepted", mode, got)
			}
		})
	}
}

func TestGuardedBrowserReadAbsenceCannotRelaxPostgresDispatchEvidenceIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "unknown_digest", "wrong_method", "incomplete_dispatch", "artifact", "opaque_write"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, call, contract, _, _ := receiptFixture(t)
			if mode != "opaque_write" {
				call.ToolName, contract.ToolName, contract.ContractRevision = "browser.submit_get_form", "browser.submit_get_form", "2"
				contract.SupportsCredentialRefs = false
				call.ArgsJson = `{"browser_session_id":"context","selector":"form","fields":[],"expected_url":"https://example.com/search","observation_version":1}`
				contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: BrowserGETVerification}
			}
			ctx := context.Background()
			grant := logicalGrant(t, s, reader, call, contract, "non-dispatch-proof")
			if err := store.ClaimGrant(ctx, grant.ID); err != nil {
				t.Fatal(err)
			}
			record, err := s.EffectForGrant(ctx, grant)
			if err != nil {
				t.Fatal(err)
			}
			e := EffectEvidence{EvidenceID: "evidence_" + randomID(), ActionID: record.ActionID, SourceActionID: grant.Binding.ActionID, SourceToolCallID: call.ToolCallId, SourceRunID: call.RunId, Source: "runtime_response", Method: record.Policy.VerificationMethod, Verdict: StateAbsent, ResultDigest: browserReadRejectionDigest("page_changed"), ObservedAt: time.Now().UTC(), RecordedAt: time.Now().UTC()}
			outcome := "dispatch_completed"
			switch mode {
			case "unknown_digest":
				e.ResultDigest = digest("invented absence")
			case "wrong_method":
				e.Method = "read_result"
			case "incomplete_dispatch":
				outcome = "unknown"
			case "artifact":
				e.ArtifactDigest = digest("invented recipient artifact")
			}
			_, err = store.RecordDispatchOutcome(ctx, grant.ID, outcome, e)
			if (err == nil) != (mode == "valid") {
				t.Fatal("PG accepted unsupported absence or rejected owning proof", mode, err)
			}
			retained, err := store.GetEffect(ctx, record.ActionID)
			if err != nil || (mode == "valid" && retained.State != StateAbsent) || (mode != "valid" && retained.State != StateExecuting) {
				t.Fatal("invalid evidence changed journal", mode, err)
			}
			if mode == "valid" && store.ClaimGrant(ctx, grant.ID) == nil {
				t.Fatal("non-dispatch proof revived consumed grant")
			}
		})
	}
}
