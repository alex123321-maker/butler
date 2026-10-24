package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type credentialWaitFlow struct{ communicationWaitFlow }

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
	for _, tool := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp"} {
		for _, scenario := range []string{"ack", "lost_ack", "read_failure", "missing_checkpoint"} {
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
