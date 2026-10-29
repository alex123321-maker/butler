package actionauth

import (
	"context"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const noInputProof = `{"preparation_rejection":"pre_input_v1","input_dispatched":false,"reason":"stale_observation"}`

func TestOwnerOutcomePreparationRejectionRequiresExactPreInputProof(t *testing.T) {
	for _, mode := range []string{"valid", "old_revision", "opaque_fill", "wrong_effect", "foreign_run", "foreign_call", "payload", "missing", "started", "null", "duplicate", "extra", "timeout", "wrong_case", "internal_error"} {
		t.Run(mode, func(t *testing.T) {
			b := ActionBinding{RunID: "run", ToolCallID: "call", ToolName: "browser.prepare_fill", Effect: EffectPrepare, ContractRevision: "3"}
			r := &toolbrokerv1.ToolResult{RunId: "run", ToolCallId: "call", ToolName: b.ToolName, Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, DetailsJson: noInputProof}}
			switch mode {
			case "old_revision":
				b.ContractRevision = "2"
			case "opaque_fill":
				b.ToolName = "browser.fill"
				r.ToolName = b.ToolName
			case "wrong_effect":
				b.Effect = EffectRead
			case "foreign_run":
				r.RunId = "other"
			case "foreign_call":
				r.ToolCallId = "other"
			case "payload":
				r.ResultJson = `{"ok":true}`
			case "missing":
				r.Error.DetailsJson = `{"reason":"stale_observation"}`
			case "started":
				r.Error.DetailsJson = `{"preparation_rejection":"pre_input_v1","input_dispatched":true,"reason":"stale_observation"}`
			case "null":
				r.Error.DetailsJson = `{"preparation_rejection":"pre_input_v1","input_dispatched":null,"reason":"stale_observation"}`
			case "duplicate":
				r.Error.DetailsJson = `{"preparation_rejection":"pre_input_v1","input_dispatched":true,"input_dispatched":false,"reason":"stale_observation"}`
			case "extra":
				r.Error.DetailsJson = `{"preparation_rejection":"pre_input_v1","input_dispatched":false,"reason":"stale_observation","secret":"not-trusted"}`
			case "timeout":
				r.Error.DetailsJson = `{"preparation_rejection":"pre_input_v1","input_dispatched":false,"reason":"timeout"}`
			case "wrong_case":
				r.Error.DetailsJson = `{"PREPARATION_REJECTION":"pre_input_v1","input_dispatched":false,"reason":"stale_observation"}`
			case "internal_error":
				r.Error.ErrorClass = commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
			}
			if (BrowserPreparationRejection(b, r) != "") != (mode == "valid") {
				t.Fatal("unsupported input absence accepted", mode)
			}
		})
	}
}

func TestOwnerOutcomePreparationRejectionPostgresIntegration(t *testing.T) {
	for _, mode := range []string{"valid", "old_revision", "opaque_fill", "missing_proof", "foreign_call", "dispatch_lost", "forged_digest", "artifact", "unknown_dispatch"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, call, contract, _, _ := receiptFixture(t)
			call.ToolName, contract.ToolName, contract.ContractRevision = "browser.prepare_fill", "browser.prepare_fill", "3"
			if mode == "old_revision" {
				contract.ContractRevision = "2"
			}
			if mode == "opaque_fill" {
				call.ToolName = "browser.fill"
				contract.ToolName = call.ToolName
				contract.ContractRevision = "3"
			}
			contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1"}
			if mode == "opaque_fill" {
				contract.EffectContract.VerificationMethod = "none"
			}
			call.ArgsJson = `{"browser_session_id":"context","expected_url":"https://external.test/form","observation_version":1,"selector":"input","value":"ordinary"}`
			g := logicalGrant(t, s, reader, call, contract, "pre-input-rejection")
			ctx := context.Background()
			if err := store.ClaimGrant(ctx, g.ID); err != nil {
				t.Fatal(err)
			}
			r := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "failed", Error: &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, DetailsJson: noInputProof}}
			if mode == "missing_proof" {
				r.Error.DetailsJson = `{"reason":"stale_observation"}`
			}
			if mode == "foreign_call" {
				r.ToolCallId = "other"
			}
			if mode == "forged_digest" || mode == "artifact" || mode == "unknown_dispatch" {
				e, err := s.EffectForGrant(ctx, g)
				if err != nil {
					t.Fatal(err)
				}
				evidence := EffectEvidence{EvidenceID: "forged", ActionID: e.ActionID, SourceActionID: g.Binding.ActionID, SourceRunID: call.RunId, SourceToolCallID: call.ToolCallId, Source: "runtime_response", Method: e.Policy.VerificationMethod, Verdict: StateAbsent, ResultDigest: preparationRejectionDigest("stale_observation")}
				outcome := "dispatch_completed"
				if mode == "forged_digest" {
					evidence.ResultDigest = digest("untrusted")
				}
				if mode == "artifact" {
					evidence.ArtifactDigest = digest("fake")
				}
				if mode == "unknown_dispatch" {
					outcome = "unknown"
				}
				if _, err = store.RecordDispatchOutcome(ctx, g.ID, outcome, evidence); err == nil {
					t.Fatal("forged absence passed PostgreSQL")
				}
				return
			}
			e, err := s.Outcome(ctx, g, r, mode == "dispatch_lost")
			want := StateUnknown
			if mode == "valid" {
				want = StateAbsent
			}
			if err != nil || e.State != want {
				t.Fatal("wrong preparation outcome", mode, e.State, err)
			}
			if err = store.ClaimGrant(ctx, g.ID); err == nil {
				t.Fatal("consumed grant revived")
			}
		})
	}
}
