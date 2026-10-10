package orchestrator

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/actionauth"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type previouslyDecidedApprovalRepo struct {
	approvals.Repository
	status string
	record approvals.Record
}

func (r *previouslyDecidedApprovalRepo) CreateApproval(_ context.Context, params approvals.CreateParams) (approvals.Record, error) {
	r.record = approvals.Record{ApprovalID: params.ApprovalID, RunID: params.RunID, SessionKey: params.SessionKey, ToolCallID: params.ToolCallID, Status: approvals.StatusPending}
	return r.record, nil
}

func (r *previouslyDecidedApprovalRepo) InsertEvent(context.Context, approvals.Event) error {
	return nil
}

func (r *previouslyDecidedApprovalRepo) GetApprovalByToolCallID(context.Context, string) (approvals.Record, error) {
	r.record.Status = r.status
	return r.record, nil
}

func TestToolApprovalUsesDurableDecision(t *testing.T) {
	for _, status := range []string{approvals.StatusRejected, approvals.StatusExpired, approvals.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			gate := NewApprovalGate()
			repo := &previouslyDecidedApprovalRepo{status: status}
			tools := &mockToolExecutor{}
			provider := &mockProvider{
				startEvents:  []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "openai", transport.ToolCallRequest{ToolCallRef: "call-1", ToolName: "http.request", ArgsJSON: `{}`})},
				submitEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "rejected"})},
			}
			delivery := &recordingDeliverySink{onApproval: func(req ApprovalRequest) error {
				gate.Resolve(req.ToolCallID, true)
				return nil
			}}
			service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
				Tools: tools, Delivery: delivery, ApprovalGate: gate,
				ActionAuthorizer: &mockApprovalChecker{toolsRequiringApproval: map[string]bool{"http.request": true}},
				ApprovalService:  approvals.NewService(repo, gate),
			}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := service.Execute(ctx, ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:1", Source: "web", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.calls) != 0 {
				t.Fatal("in-memory approval overrode durable denial")
			}
			if len(provider.submittedResults) != 1 || !strings.Contains(provider.submittedResults[0].ToolResultJSON, "rejected") {
				t.Fatal("model did not receive rejection")
			}
		})
	}
}

type unavailableApprovalChecker struct{}

func (unavailableApprovalChecker) PrepareAction(context.Context, *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	return actionauth.PreparedAction{}, errors.New("policy service unavailable")
}
func (unavailableApprovalChecker) AuthorizeAction(context.Context, *toolbrokerv1.ToolCall, string, string, string) (string, error) {
	return "", errors.New("policy service unavailable")
}

func (unavailableApprovalChecker) RequiresApproval(context.Context, string) (bool, error) {
	return false, errors.New("policy service unavailable")
}

func TestToolAuthorizationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		checker ActionAuthorizer
		want    string
	}{
		{"missing policy", nil, "exact action authorizer is not configured"},
		{"failed policy", unavailableApprovalChecker{}, "policy service unavailable"},
		{"missing approval gate", &mockApprovalChecker{toolsRequiringApproval: map[string]bool{"http.request": true}}, "approval gate is not configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, batch := range []bool{false, true} {
				tools := &mockToolExecutor{result: &toolbrokerv1.ToolResult{Status: "completed"}}
				call := transport.ToolCallRequest{ToolCallRef: "call-1", ToolName: "http.request", ArgsJSON: `{"url":"https://example.com"}`}
				event := transport.NewToolCallRequestedEvent("", "openai", call)
				if batch {
					event.EventType = transport.EventTypeToolCallBatchRequested
					event.ToolCall = nil
					event.ToolCallBatch = &transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{call}}
				}
				provider := &mockProvider{startEvents: []transport.TransportEvent{event}}
				service := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{Tools: tools, ActionAuthorizer: tc.checker}, nil)
				_, err := service.Execute(context.Background(), ingress.InputEvent{EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:1", Source: "web", PayloadJSON: `{"text":"fetch"}`, CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1"})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("batch=%v: expected %q, got %v", batch, tc.want, err)
				}
				if len(tools.calls) != 0 || len(provider.submittedResults) != 0 {
					t.Fatalf("batch=%v: unauthorized tool executed or resumed model", batch)
				}
			}
		})
	}
}
