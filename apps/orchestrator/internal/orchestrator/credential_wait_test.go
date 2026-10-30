package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type credentialStoppedFlow struct {
	credentialWaitFlow
	suspends int
}

func (f *credentialStoppedFlow) ExecuteAuthorizedToolCall(_ context.Context, c *toolbrokerv1.ToolCall, _ string) (*toolbrokerv1.ToolResult, error) {
	f.dispatches++
	return &toolbrokerv1.ToolResult{ToolName: c.ToolName, ToolCallId: c.ToolCallId, RunId: c.RunId, Status: "execution_stopped", Error: &toolbrokerv1.ToolError{DetailsJson: `{"reason":"external_effect_unknown"}`}}, nil
}
func (f *credentialStoppedFlow) SuspendRequest(context.Context, string, string) (string, error) {
	f.suspends++
	return "", errors.New("no request was dispatched")
}
func TestCredentialRequestBudgetStopKeepsItsReason(t *testing.T) {
	for _, tool := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp", "credentials.request_login"} {
		t.Run(tool, func(t *testing.T) {
			runs := newMemoryRunManager()
			f := &credentialStoppedFlow{credentialWaitFlow: credentialWaitFlow{communicationWaitFlow{runs: runs}}}
			p := &communicationWaitProvider{mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "fixture", transport.ToolCallRequest{ToolCallRef: "credential", ToolName: tool, ArgsJSON: `{}`})}}}
			history := &memoryTranscriptStore{}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, history, p, Config{Credentials: f, Tools: f, Checkpoints: f, ActionAuthorizer: &mockApprovalChecker{}}, nil)
			r, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "input", IdempotencyKey: "input", SessionKey: "web:waiting", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"sign in"}`, CreatedAt: time.Now()})
			if err != nil || r == nil || r.ReceiptStatus != "blocked" || r.ErrorCode != "external_effect_unknown" || f.suspends != 0 || f.stage != "execution_stopped" || f.dispatches != 1 || len(history.toolCalls) != 1 {
				t.Fatal("definite budget stop became an unproved credential wait", r, err, f.suspends, f.stage)
			}
		})
	}
}

type credentialWaitFlow struct{ communicationWaitFlow }

func (f *credentialWaitFlow) ExecuteAuthorizedToolCall(ctx context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	r, err := f.communicationWaitFlow.ExecuteAuthorizedToolCall(ctx, c, grant)
	if f.scenario == "unknown_ack" && r != nil {
		r.Status = "effect_unknown"
	}
	return r, err
}

func (f *credentialWaitFlow) SuspendRequest(context.Context, string, string) (string, error) {
	if f.scenario == "read_failure" || f.scenario == "missing_checkpoint" {
		return "", errors.New("fixture checkpoint unavailable")
	}
	return "creq_fixture", nil
}

func (*credentialWaitFlow) ResumeInteraction(context.Context, string, string) (string, error) {
	return "", errors.New("fixture response unavailable")
}

func TestCredentialRequestsPreservePrivateWaitAcrossLostAcknowledgement(t *testing.T) {
	for _, tool := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp", "credentials.request_login"} {
		for _, scenario := range []string{"ack", "lost_ack", "unknown_ack", "read_failure", "missing_checkpoint"} {
			t.Run(tool+"/"+scenario, func(t *testing.T) {
				runs := newMemoryRunManager()
				flow := &credentialWaitFlow{communicationWaitFlow{runs: runs, scenario: scenario}}
				provider := &communicationWaitProvider{mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "fixture", transport.ToolCallRequest{ToolCallRef: "credential", ToolName: tool, ArgsJSON: `{"browser_session_id":"browser","expected_url":"https://example.com/login","purpose":"Sign in"}`})}}}
				history := &memoryTranscriptStore{}
				s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, history, provider, Config{Credentials: flow, Tools: flow, Checkpoints: flow, ActionAuthorizer: &mockApprovalChecker{}}, nil)
				r, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "input", IdempotencyKey: "input", SessionKey: "web:waiting", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"sign in"}`, CreatedAt: time.Now()})
				if scenario == "read_failure" || scenario == "missing_checkpoint" {
					if !errors.Is(err, ErrCommunicationStatePending) || r != nil {
						t.Fatal("checkpoint uncertainty lost durable wait", r, err)
					}
				} else if err != nil || r.ReceiptStatus != "waiting_for_user" {
					t.Fatal("credential request did not suspend", r, err)
				}
				if flow.dispatches != 1 || len(history.toolCalls) != 0 || len(provider.submittedResults) != 0 || !provider.cancelled {
					t.Fatal("private wait replayed dispatch, leaked result or retained old provider context")
				}
			})
		}
	}
}
