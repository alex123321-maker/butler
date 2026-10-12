package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/internal/actionauth"
)

func TestOwnerApprovalHTTPProjectionOmitsPrivateStoredMaterial(t *testing.T) {
	binding := actionauth.ActionBinding{ActionID: "action-owner", RunID: "run-owner", ToolCallID: "call-owner", ToolName: "http.request", TaskID: "task-owner", TaskRevision: 4, ConstraintsRevision: 3, ContractHash: strings.Repeat("a", 64), ArgsHash: strings.Repeat("b", 64), Effect: actionauth.EffectCommit, Resource: "https://user:PRIVATE_PASSWORD@example.invalid/account?token=PRIVATE_QUERY"}
	details, _ := json.Marshal(map[string]any{"action_binding": binding, "hidden_reasoning": "PRIVATE_REASONING", "continuation": "PRIVATE_CONTINUATION"})
	record := approvals.Record{ApprovalID: "approval-owner", RunID: binding.RunID, ToolCallID: binding.ToolCallID, ToolName: binding.ToolName, Status: approvals.StatusPending, ArgsJSON: `{"password":"PRIVATE_SECRET","credential_ref":"PRIVATE_ALIAS"}`, PayloadJSON: `{"body":"PRIVATE_BODY"}`, Summary: "PRIVATE_SUMMARY", DetailsJSON: string(details), ResolutionReason: "PRIVATE_RESOLUTION"}
	server := NewApprovalsServer(&fakeApprovalsStore{items: []approvals.Record{record}, itemByID: map[string]approvals.Record{record.ApprovalID: record}}, nil, nil)
	for _, route := range []struct {
		path    string
		handler http.Handler
	}{{"/api/v2/approvals", server.HandleListApprovals()}, {"/api/v2/approvals/approval-owner", server.HandleGetApproval("/api/v2/approvals/")}} {
		response := httptest.NewRecorder()
		route.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route.path, nil))
		if response.Code != 200 {
			t.Fatalf("HTTP projection failed: %d", response.Code)
		}
		body := response.Body.String()
		for _, private := range []string{"PRIVATE_", "args_json", "payload_json", "details_json", "hidden_reasoning", "continuation"} {
			if strings.Contains(body, private) {
				t.Fatalf("private field escaped actual HTTP response: %s", private)
			}
		}
		if !strings.Contains(body, `"task_revision":4`) || !strings.Contains(body, "https://example.invalid/account") {
			t.Fatal("exact safe owner consequences were lost")
		}
	}
}
