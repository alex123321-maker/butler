package executionnotice

import (
	"strings"
	"testing"
)

func TestExecutionNoticeClassification(t *testing.T) {
	for _, tt := range []struct {
		status, checkpoint, errorCode, approval string
		result                                  bool
		code, reason                            string
	}{
		{"queued", "accepted", "", "", false, "queued", ""},
		{"running", "model_budget_reserved", "", "", false, "working", ""},
		{"waiting_for_user", "awaiting_approval", "", "approval:1", false, "approval_required", ""},
		{"waiting_for_user", "awaiting_interaction", "", "", false, "waiting_for_user", ""},
		{"blocked", "execution_stopped", "budget_provider_input_bytes_conversation", "", false, "blocked", "budget"},
		{"blocked", "model_budget_reserved", "provider_unavailable", "", false, "blocked", "model_unavailable"},
		{"blocked", "tool_dispatching", "effect_unknown", "", false, "blocked", "uncertain_action"},
		{"failed", "model_budget_reserved", "execution_deadline_exceeded", "", false, "failed", "timeout"},
		{"failed", "prepared", "arbitrary-password-query-raw-provider-error", "", false, "failed", "execution_failed"},
		{"completed", "finalizing", "", "", true, "finished", ""},
		{"completed", "finalizing", "", "", false, "missing_response", ""},
	} {
		t.Run(tt.status+tt.errorCode+tt.approval, func(t *testing.T) {
			c, r := classify(snapshot{Status: tt.status, Checkpoint: tt.checkpoint, ErrorCode: tt.errorCode, ApprovalRef: tt.approval, HasResult: tt.result})
			if c != tt.code || r != tt.reason {
				t.Fatalf("got %s/%s want %s/%s", c, r, tt.code, tt.reason)
			}
			text := Text(c, r)
			if text == "" || strings.Contains(text, tt.errorCode) && tt.errorCode != "" {
				t.Fatal("missing or reflected notice", text)
			}
		})
	}
	if Text("model-authored-text", "anything") != "" {
		t.Fatal("unknown notice accepted")
	}
}

func TestExecutionNoticeQueuePredecessor(t *testing.T) {
	for _, state := range []string{"blocked", "waiting_for_user", "running", "queued", "", "private-error-text"} {
		code, reason := classify(snapshot{Status: "queued", PredecessorState: state})
		blocked := state == "blocked" || state == "waiting_for_user"
		if (code == "queue_blocked") != blocked || Text(code, reason) == "" {
			t.Fatal("queue dependency classification lost", state, code, reason)
		}
		code, _ = classify(snapshot{Status: "running", PredecessorState: state})
		if code != "working" {
			t.Fatal("old dependency replaced current execution")
		}
	}
	if Text("queue_blocked", "private-error-text") != "" {
		t.Fatal("arbitrary queue reason accepted")
	}
}
