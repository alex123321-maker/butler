package approval

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
)

func TestOwnerApprovalFactsProjectExactConsequencesWithoutPrivateDetails(t *testing.T) {
	b := actionauth.ActionBinding{ActionID: "action-fixture", RunID: "run-fixture", ToolCallID: "call-fixture", ToolName: "http.request", Effect: actionauth.EffectCommit, Resource: "https://user:PRIVATE_SECRET@example.invalid/account?token=PRIVATE_SECRET#PRIVATE_SECRET", ContractHash: strings.Repeat("a", 64), ArgsHash: strings.Repeat("b", 64), TaskID: "task-fixture", TaskRevision: 4, ConstraintsRevision: 3, GoalID: "goal-fixture", GoalRevision: 2, MandateID: "mandate-fixture", MandateRevision: 5, CredentialRefs: []actionauth.CredentialRef{{Alias: "PRIVATE_SECRET", Field: "password"}}}
	raw, _ := json.Marshal(map[string]any{"action_binding": b, "continuation": "PRIVATE_SECRET", "hidden_reasoning": "PRIVATE_SECRET"})
	r := Record{RunID: b.RunID, ToolCallID: b.ToolCallID, ToolName: b.ToolName, ArgsJSON: `{"password":"PRIVATE_SECRET"}`, DetailsJSON: string(raw)}
	f := OwnerFactsForRecord(r)
	if f == nil || f.Resource != "https://example.invalid/account" || f.Effect != "commit" || f.TaskRevision != 4 || f.ConstraintsRevision != 3 || f.MandateRevision != 5 || f.CredentialReferenceCount != 1 {
		t.Fatalf("facts unavailable/inaccurate: %#v", f)
	}
	encoded, _ := json.Marshal(f)
	if strings.Contains(string(encoded), "PRIVATE_SECRET") || strings.Contains(string(encoded), "continuation") || strings.Contains(string(encoded), "hidden_reasoning") {
		t.Fatal("private binding/args/body escaped safe facts")
	}
	b.Resource = "/srv/private/profile/import.json"
	raw, _ = json.Marshal(map[string]any{"action_binding": b})
	r.DetailsJSON = string(raw)
	if f = OwnerFactsForRecord(r); f == nil || f.Resource != "" {
		t.Fatal("private filesystem path projected")
	}
	r.RunID = "other-run"
	if OwnerFactsForRecord(r) != nil {
		t.Fatal("facts taken from unrelated binding")
	}
}
