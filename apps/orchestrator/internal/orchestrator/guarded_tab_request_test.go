package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/butler/butler/internal/transport"
)

// These are core flow unit fixtures. Actual private authority, ingress claim,
// Broker journal/budget and protected owner decisions have separate PG proofs.
type guardedFlowFixture struct {
	runs     *memoryRunManager
	stage    string
	calls    int
	lost     bool
	received bool
}

func (f *guardedFlowFixture) ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	return nil, errors.New("raw route denied")
}
func (f *guardedFlowFixture) ExecuteAuthorizedToolCall(_ context.Context, call *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	if grant == "" {
		return nil, errors.New("grant required")
	}
	f.calls++
	f.runs.records[call.RunId].CurrentState = commonv1.RunState_RUN_STATE_AWAITING_APPROVAL
	f.runs.records[call.RunId].Status = "awaiting_approval"
	f.stage = "awaiting_approval"
	if f.lost {
		return nil, errors.New("lost private ACK")
	}
	return &toolbrokerv1.ToolResult{Status: "completed", ResultJson: `{"approval_id":"opaque-choice","state":"awaiting_owner_choice"}`}, nil
}
func (f *guardedFlowFixture) PendingRequest(_ context.Context, runID, callID, effectID string) (ApprovalRequest, error) {
	if f.stage != "awaiting_approval" || callID != "guarded-call" || effectID != "guarded-effect" {
		return ApprovalRequest{}, errors.New("stored request mismatch")
	}
	f.received = true
	return ApprovalRequest{RunID: runID, SessionKey: f.runs.records[runID].SessionKey, ToolCallID: callID, ApprovalID: "opaque-choice", ApprovalType: "browser_tab_selection", ToolName: singletabcontrol.BindTool}, nil
}
func (f *guardedFlowFixture) SaveCheckpoint(_ context.Context, _ string, stage, _ string) error {
	f.stage = stage
	return nil
}
func (f *guardedFlowFixture) LoadCheckpoint(context.Context, string) (string, string, error) {
	return f.stage, `{}`, nil
}

type guardedFlowAuthorizer struct{ mockApprovalChecker }

func (*guardedFlowAuthorizer) PrepareAction(_ context.Context, call *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	return actionauth.PreparedAction{Binding: actionauth.ActionBinding{ActionID: "guarded-action", ActionHash: "fixture-hash", RunID: call.RunId, ToolCallID: call.ToolCallId, ToolName: call.ToolName}, EffectID: "guarded-effect", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func TestGuardedTabRequestCoreSuspendsBeforeGenericResult(t *testing.T) {
	for _, scenario := range []string{"completed_ack", "lost_ack", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			runs := newMemoryRunManager()
			f := &guardedFlowFixture{runs: runs, lost: scenario == "lost_ack"}
			provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "fixture", transport.ToolCallRequest{ToolCallRef: "guarded-call", ToolName: singletabcontrol.BindTool, ArgsJSON: "{}"})}}
			history := &memoryTranscriptStore{}
			delivery := &recordingDeliverySink{}
			cfg := Config{ProviderName: "fixture", ModelName: "fixture", Tools: f, Checkpoints: f, ActionAuthorizer: &guardedFlowAuthorizer{}, Delivery: delivery}
			if scenario != "disabled" {
				cfg.GuardedTabRequests = f
			}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, history, provider, cfg, nil)
			result, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "guarded-input", IdempotencyKey: "guarded-input", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:guarded", Source: "web", PayloadJSON: `{"text":"ask owner"}`, CreatedAt: time.Now().UTC()})
			if scenario == "disabled" {
				if f.calls != 0 || err == nil {
					t.Fatal("disabled composition dispatched request")
				}
				return
			}
			if err != nil || result.ReceiptStatus != "waiting_for_user" || result.CurrentState != commonv1.RunState_RUN_STATE_AWAITING_APPROVAL || !f.received || f.calls != 1 {
				t.Fatal("core did not retain durable suspension", result, err)
			}
			if f.stage != "awaiting_approval" || len(history.toolCalls) != 0 || len(provider.submittedResults) != 0 {
				t.Fatal("generic result/checkpoint overwrote selection continuation")
			}
		})
	}
}
