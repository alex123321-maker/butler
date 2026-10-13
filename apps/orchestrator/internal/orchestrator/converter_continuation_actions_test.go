package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"strings"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/security/content"
)

func TestConverterContinuationConfirmedPriorActionsDataAndReload(t *testing.T) {
	r, s, ctx, event, p := activatedContextFixture()
	r.value.PriorRunID = "completed-candidate-phase"
	r.value.ConfirmedPriorActions = []ConverterContinuationAction{{RunID: r.value.PriorRunID, ToolCallID: "prepared", ToolName: "extension.project.prepare-patch", ActionID: "real-action", EffectID: "real-effect", Effect: actionauth.EffectCommit, ArgsHash: strings.Repeat("a", 64), Args: json.RawMessage(`{"url":"https://owned.invalid/project/patch","body":{"project_revision":"current"}}`), Result: json.RawMessage(`{"body":"{\"artifact_id\":\"already-prepared\"}"}`)}}
	if err := s.attachConverterContinuation(ctx, event, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.InputItems) != 3 || !strings.Contains(p.InputItems[1].Content, "Reuse their existing artifacts") || content.ItemOrigin(p.InputItems[2]).IsInstruction() || content.ItemOrigin(p.InputItems[2]).Kind != content.SubagentOutput {
		t.Fatal("prior result became instruction/transport tool response")
	}
	var data struct {
		Data struct {
			Actions   []ConverterContinuationAction `json:"confirmed_prior_actions"`
			Verified  bool                          `json:"original_task_verified"`
			Authority bool                          `json:"instruction_authority"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(p.InputItems[2].Content), &data) != nil || len(data.Data.Actions) != 1 || data.Data.Actions[0].EffectID != "real-effect" || data.Data.Verified || data.Data.Authority {
		t.Fatal("confirmed receipt identity or task-verification boundary lost")
	}
	durable := durableSourcePreparation(p)
	if strings.Contains(durable.InputItems[2].Content, "already-prepared") || strings.Contains(durable.InputItems[2].Content, "confirmed_prior_actions") {
		t.Fatal("receipt cached instead of private source reload")
	}
	r.value.ConfirmedPriorActions[0].Result = json.RawMessage(`{"body":"{\"artifact_id\":\"current-checked-receipt\"}"}`)
	for range 2 {
		if err := s.rehydrateSourcePreparation(ctx, &durable); err != nil {
			t.Fatal(err)
		}
	}
	if len(durable.InputItems) != 3 || strings.Contains(durable.InputItems[2].Content, "already-prepared") || !strings.Contains(durable.InputItems[2].Content, "current-checked-receipt") || r.calls != 3 {
		t.Fatal("recovery duplicated/staled prior facts or bypassed current reader")
	}
	r.deny = true
	if err := s.rehydrateSourcePreparation(ctx, &durable); err == nil {
		t.Fatal("revoked current proof retained prior work")
	}
}

func TestConverterContinuationConfirmedPriorActionsMalformedDenied(t *testing.T) {
	for _, kind := range []string{"foreign_run", "missing_effect", "bad_args_hash", "malformed_result", "duplicate_call", "too_many", "oversized", "candidate_phase"} {
		t.Run(kind, func(t *testing.T) {
			r, s, ctx, event, p := activatedContextFixture()
			r.value.PriorRunID = "prior"
			a := ConverterContinuationAction{RunID: "prior", ToolCallID: "call", ToolName: "extension.project.prepare-patch", ActionID: "action", EffectID: "effect", Effect: actionauth.EffectCommit, ArgsHash: strings.Repeat("a", 64), Args: json.RawMessage(`{}`), Result: json.RawMessage(`{"artifact_id":"known"}`)}
			r.value.ConfirmedPriorActions = []ConverterContinuationAction{a}
			switch kind {
			case "foreign_run":
				r.value.ConfirmedPriorActions[0].RunID = "other"
			case "missing_effect":
				r.value.ConfirmedPriorActions[0].EffectID = ""
			case "bad_args_hash":
				r.value.ConfirmedPriorActions[0].ArgsHash = strings.Repeat("z", 64)
			case "malformed_result":
				r.value.ConfirmedPriorActions[0].Result = json.RawMessage(`{`)
			case "duplicate_call":
				r.value.ConfirmedPriorActions = append(r.value.ConfirmedPriorActions, a)
			case "too_many":
				for range 32 {
					r.value.ConfirmedPriorActions = append(r.value.ConfirmedPriorActions, a)
				}
			case "oversized":
				r.value.ConfirmedPriorActions[0].Result = json.RawMessage(`"` + strings.Repeat("x", 256<<10) + `"`)
			case "candidate_phase":
				r.value.Purpose = generatedtools.CandidateReturn
				r.value.Activation = nil
			}
			if err := s.attachConverterContinuation(ctx, event, &p); err == nil {
				t.Fatal("malformed/foreign prior work entered model context")
			}
		})
	}
}
