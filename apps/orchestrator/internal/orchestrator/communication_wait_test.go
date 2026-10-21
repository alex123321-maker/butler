package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type communicationWaitFlow struct {
	communicationKernelFixture
	runs       *memoryRunManager
	stage      string
	scenario   string
	dispatches int
}

type communicationWaitProvider struct {
	mockProvider
	cancelled bool
}

func (p *communicationWaitProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled = true
	return nil, nil
}

func (f *communicationWaitFlow) PendingInteraction(context.Context, string) (string, error) {
	if f.scenario == "read_failure" {
		return "", errors.New("fixture durable read unavailable")
	}
	if f.scenario == "missing_checkpoint" {
		return "", nil
	}
	return "interaction:fixture", nil
}
func (f *communicationWaitFlow) SaveCheckpoint(_ context.Context, _ string, stage, _ string) error {
	f.stage = stage
	return nil
}
func (f *communicationWaitFlow) LoadCheckpoint(context.Context, string) (string, string, error) {
	return f.stage, `{}`, nil
}
func (*communicationWaitFlow) ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	return nil, errors.New("raw route denied")
}
func (f *communicationWaitFlow) ExecuteAuthorizedToolCall(_ context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	if grant == "" {
		return nil, errors.New("grant required")
	}
	f.dispatches++
	if f.scenario != "missing_checkpoint" {
		f.stage = "awaiting_interaction"
		f.runs.records[c.RunId].CurrentState = commonv1.RunState_RUN_STATE_AWAITING_APPROVAL
		f.runs.records[c.RunId].Status = "awaiting_approval"
	}
	if f.scenario == "lost_ack" {
		return nil, errors.New("fixture lost private ACK")
	}
	return &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"status":"accepted","publication_ref":"publication:fixture","interaction_ref":"interaction:fixture"}`}, nil
}

func TestCommunicationWidgetReadFailurePreservesSuspension(t *testing.T) {
	for _, scenario := range []string{"ack", "lost_ack", "read_failure", "missing_checkpoint"} {
		t.Run(scenario, func(t *testing.T) {
			runs := newMemoryRunManager()
			f := &communicationWaitFlow{runs: runs, scenario: scenario}
			provider := &communicationWaitProvider{mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "fixture", transport.ToolCallRequest{ToolCallRef: "question", ToolName: "communication.widget", ArgsJSON: `{"schema_version":1,"question":"Choose","input":{"type":"text"}}`})}}}
			history := &memoryTranscriptStore{}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, history, provider, Config{Communication: f, Tools: f, Checkpoints: f, ActionAuthorizer: &mockApprovalChecker{}}, nil)
			r, e := s.Execute(context.Background(), ingress.InputEvent{EventID: "input", IdempotencyKey: "input", SessionKey: "web:waiting", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"ask owner"}`, CreatedAt: time.Now()})
			if scenario == "read_failure" || scenario == "missing_checkpoint" {
				if !errors.Is(e, ErrCommunicationStatePending) || r != nil {
					t.Fatal("unproved wait resumed/failed the run", r, e)
				}
			} else if e != nil || r.ReceiptStatus != "waiting_for_user" {
				t.Fatal("durable widget did not suspend", r, e)
			}
			if f.dispatches != 1 || len(history.toolCalls) != 0 || len(provider.submittedResults) != 0 {
				t.Fatal("generic result or repeated dispatch escaped suspension")
			}
			if !provider.cancelled {
				t.Fatal("old provider context retained across suspension")
			}
			want := "awaiting_interaction"
			if scenario == "missing_checkpoint" {
				want = "tool_dispatching"
			}
			if f.stage != want {
				t.Fatal("trusted checkpoint overwritten", f.stage, want)
			}
			for _, record := range runs.records {
				if record.CurrentState == commonv1.RunState_RUN_STATE_FAILED {
					t.Fatal("temporary state read poisoned durable wait")
				}
			}
		})
	}
}
