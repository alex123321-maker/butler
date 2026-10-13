package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func cloudV3PGFixture(e cloudV3WindowEnvelope) CloudV3PGWitness {
	w := CloudV3PGWitness{Version: CloudV3PGWitnessVersion, Window: CloudV3PGWindow{e.Total, e.History.SHA, e.Pin.PolicySHA256, e.Expires}, Attempts: []CloudV3PGAttempt{}}
	for _, a := range e.Ledger {
		w.Attempts = append(w.Attempts, CloudV3PGAttempt{a.Ordinal, a.Phase, a.CaseID, a.Role, a.Context, a.InputSHA, a.Created, a.Outcome, a.InputTokens, a.OutputTokens, a.TotalTokens, a.OutputSHA, a.ParentContextID})
	}
	return w
}

func TestCloudV3PGCorrelationMatchesLiteralZeroUnknownAndFailedPartialWithoutQualityClaim(t *testing.T) {
	_, _, _, e, _ := cloudV3WindowFixture(t)
	e.Status, e.New, e.Total = "failed", 3, 251
	e.Attempts, e.Ledger = e.Attempts[:3], e.Ledger[:3]
	e.Single.Tasks, e.Multi.Tasks = []CloudComparisonTask{}, []CloudComparisonTask{}
	e.Ledger[0].InputTokens, e.Ledger[0].OutputTokens, e.Ledger[0].TotalTokens = intCloud(0), intCloud(0), intCloud(0)
	e.Ledger[1].Outcome, e.Ledger[1].OutputSHA = "transport_error", nil
	e.Ledger[2].Outcome, e.Ledger[2].OutputSHA = "unavailable", nil
	e.Ledger[2].InputTokens, e.Ledger[2].OutputTokens, e.Ledger[2].TotalTokens = nil, nil, nil
	w := cloudV3PGFixture(e)
	witness, _ := json.Marshal(w)
	r, err := CorrelateCloudV3PGWitness(cloudV3WindowBytes(t, e), witness)
	if err != nil || !r.Matched || r.TotalAttempts != 251 || r.Rows != 3 || r.QualityPassed || r.InferenceProven || r.Execution {
		t.Fatal("partial journal correlation lost literal measurements", err, r)
	}
	encoded, _ := json.Marshal(r)
	for _, forbidden := range []string{"case_id", "context_id", "output", "input_tokens", "quality_attempts", "documents", "golden"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("content in correlation projection", forbidden)
		}
	}
	w.Attempts[2].TotalTokens = intCloud(0)
	witness, _ = json.Marshal(w)
	r, err = CorrelateCloudV3PGWitness(cloudV3WindowBytes(t, e), witness)
	if err != nil || r.Matched || r.TotalAttempts != 251 {
		t.Fatal("NULL replaced by known zero", err, r)
	}
}

func TestCloudV3PGCorrelationRejectsEveryJournalMutationExtraOrMissingRow(t *testing.T) {
	_, _, _, e, _ := cloudV3WindowFixture(t)
	for _, name := range []string{"counter", "expiry", "history", "policy", "role", "phase", "case", "context", "parent", "single_parent", "input", "output", "outcome", "usage", "missing", "reorder", "alias", "duplicate", "missing_parent_field"} {
		t.Run(name, func(t *testing.T) {
			w := cloudV3PGFixture(e)
			switch name {
			case "counter":
				w.Window.Attempts--
			case "expiry":
				w.Window.Expires = w.Window.Expires.Add(1)
			case "history":
				w.Window.BaselineHash = strings.Repeat("f", 64)
			case "policy":
				w.Window.PolicyHash = strings.Repeat("f", 64)
			case "role":
				w.Attempts[236].Role = CloudRoleChildB
			case "phase":
				w.Attempts[0].Phase = "comparison"
			case "case":
				w.Attempts[0].CaseID = "foreign"
			case "context":
				w.Attempts[0].Context = "shadow-" + strings.Repeat("f", 32)
			case "parent":
				w.Attempts[237].ParentContextID = stringCloud("shadow-" + strings.Repeat("f", 32))
			case "single_parent":
				w.Attempts[0].ParentContextID = w.Attempts[237].ParentContextID
			case "input":
				w.Attempts[0].InputHash = strings.Repeat("f", 64)
			case "output":
				w.Attempts[0].OutputHash = stringCloud(strings.Repeat("f", 64))
			case "outcome":
				w.Attempts[0].Outcome, w.Attempts[0].OutputHash = "transport_error", nil
			case "usage":
				w.Attempts[0].TotalTokens = nil
			case "missing":
				w.Attempts = w.Attempts[:len(w.Attempts)-1]
			case "reorder":
				w.Attempts[0], w.Attempts[1] = w.Attempts[1], w.Attempts[0]
			}
			b, _ := json.Marshal(w)
			if name == "alias" {
				b = []byte(strings.Replace(string(b), `"phase":`, `"Phase":`, 1))
			} else if name == "duplicate" {
				b = []byte(strings.Replace(string(b), `"ordinal":249`, `"ordinal":249,"\u006frdinal":250`, 1))
			} else if name == "missing_parent_field" {
				b = []byte(strings.Replace(string(b), `,"parent_context_id":null`, "", 1))
			}
			r, err := CorrelateCloudV3PGWitness(cloudV3WindowBytes(t, e), b)
			if err == nil && r.Matched || r.QualityPassed || r.Execution {
				t.Fatal("journal mutation accepted", name)
			}
		})
	}
}
