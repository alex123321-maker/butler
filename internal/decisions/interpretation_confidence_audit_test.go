package decisions

// Offline secondary analysis only. It does not resample the frozen holdout or
// replace its original confidence report. No model/provider call is made.
import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The API can emit one token " true"/" false" instead of "true"/"false".
// Condition only on exactly the selected whitespace prefix. Other spellings
// are not summed, inferred or treated as duplicated semantic probabilities.
func interpretationQwenWhitespaceScores(original qwenResponse) (Interpretation, error) {
	r := original
	r.LogProbs = append(r.LogProbs[:0:0], original.LogProbs...)
	for i, p := range original.LogProbs {
		canonical := strings.TrimSpace(p.Token)
		if canonical != "true" && canonical != "false" {
			continue
		}
		prefix := strings.TrimSuffix(p.Token, canonical)
		if strings.TrimSpace(prefix) != "" {
			continue
		}
		r.LogProbs[i].Token = canonical
		r.LogProbs[i].Top = nil
		for _, option := range p.Top {
			if option.Token == prefix+"true" || option.Token == prefix+"false" {
				option.Token = strings.TrimPrefix(option.Token, prefix)
				r.LogProbs[i].Top = append(r.LogProbs[i].Top, option)
			}
		}
	}
	return interpretationQwenAnswer(r)
}

func TestInterpretationBooleanWhitespaceDoesNotInventAlternatives(t *testing.T) {
	r := qwenResponse{Response: `{"request_action":true,"change_constraint":false,"stop_work":false,"ask_information":false,"provide_information":false,"modality":0,"target":1}`, Done: true}
	// An absent prefix-matched alternative must still keep confidence unknown.
	raw := []byte(`{"response":"{\"request_action\":true,\"change_constraint\":false,\"stop_work\":false,\"ask_information\":false,\"provide_information\":false,\"modality\":0,\"target\":1}","done":true,"logprobs":[{"token":" true","logprob":-0.1,"top_logprobs":[{"token":" true","logprob":-0.1},{"token":"false","logprob":-2}]}]}`)
	if json.Unmarshal(raw, &r) != nil {
		t.Fatal("fixture")
	}
	o, err := interpretationQwenWhitespaceScores(r)
	if err != nil || !o.ConfidenceUnknown || !o.Escalate || !sameIntents(o.Intents, []string{"request_action"}) {
		t.Fatal(o, err)
	}
}

func TestOfflineInterpretationConfidenceAudit(t *testing.T) {
	input, output := os.Getenv("BUTLER_TEST_INTERPRETATION_AUDIT_INPUT"), os.Getenv("BUTLER_TEST_INTERPRETATION_AUDIT_OUTPUT")
	if input == "" || output == "" {
		t.Skip("explicit saved-output offline audit only")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Version string `json:"report_version"`
		Cases   []struct {
			Case         string             `json:"case"`
			Window       int                `json:"window"`
			Profile      string             `json:"profile"`
			Error        string             `json:"error"`
			Gold         interpretationGold `json:"gold"`
			Observation  Interpretation     `json:"observation"`
			QwenResponse *qwenResponse      `json:"qwen_response"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &report) != nil || report.Version != "interpretation-comparison-v3" {
		t.Fatal("invalid saved report")
	}
	rows := []map[string]any{}
	observed, known, nonEscalated, wrong := 0, 0, 0, 0
	for _, row := range report.Cases {
		if row.Profile != interpretationQwenID || row.Error != "" || row.QwenResponse == nil {
			continue
		}
		observed++
		o, err := interpretationQwenWhitespaceScores(*row.QwenResponse)
		if err != nil {
			t.Fatal("saved scores cannot be correlated", row.Case, err)
		}
		if !sameIntents(o.Intents, row.Observation.Intents) || o.Modality != row.Observation.Modality || o.Target != row.Observation.Target {
			t.Fatal("offline audit changed a prediction", row.Case)
		}
		if !o.ConfidenceUnknown {
			known++
		}
		joint := sameIntents(o.Intents, row.Gold.Intents) && o.Modality == row.Gold.Modality && o.Target == row.Gold.Target
		if !o.Escalate {
			nonEscalated++
			if !joint {
				wrong++
			}
		}
		rows = append(rows, map[string]any{"case": row.Case, "window": row.Window, "original_confidence_unknown": row.Observation.ConfidenceUnknown, "repaired_confidence": o, "joint_correct": joint})
	}
	writeInterpretationReport(t, output, map[string]any{"version": "interpretation-v3-offline-confidence-prefix-audit", "secondary_analysis": true, "new_model_calls": 0, "all_semantic_predictions_unchanged": true, "observed": observed, "complete_conditional_scores": known, "non_escalated": nonEscalated, "non_escalated_wrong": wrong, "primary_report_unchanged": true, "thresholds": map[string]float64{"score": .65, "margin": .15}, "limitations": "conditional scores at a fixed observed whitespace prefix remain uncalibrated; this is a post-hoc protocol audit, not a new independent confidence screen", "cases": rows})
}
