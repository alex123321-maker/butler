package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

var ErrApprovalSuspended = errors.New("execution suspended for durable owner decision")

type approvalContinuation struct {
	Version  int                        `json:"version"`
	Prepared preparedRun                `json:"prepared"`
	ToolCall *transport.ToolCallRequest `json:"tool_call"`
	Binding  actionauth.ActionBinding   `json:"binding"`
}
type recoveredApprovalKey struct{}

func (s *Service) suspendApproval(ctx context.Context, current *sessionv1.RunRecord, requested *transport.ToolCallRequest, action actionauth.PreparedAction) (*sessionv1.RunRecord, error) {
	if s.config.Checkpoints == nil || s.config.ApprovalService == nil {
		return current, fmt.Errorf("durable input/approval service required")
	}
	_, payload, err := s.config.Checkpoints.LoadCheckpoint(ctx, current.GetRunId())
	if err != nil {
		return current, err
	}
	var prepared preparedRun
	if err = json.Unmarshal([]byte(payload), &prepared); err != nil {
		return current, err
	}
	continuation := approvalContinuation{Version: 1, Prepared: prepared, ToolCall: requested, Binding: action.Binding}
	expires := action.ExpiresAt
	if expires.IsZero() {
		expires = time.Now().UTC().Add(15 * time.Minute)
	}
	record, err := s.config.ApprovalService.CreatePendingApproval(ctx, approvals.CreatePendingParams{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), ToolCallID: action.Binding.ToolCallID, ToolName: requested.ToolName, ArgsJSON: requested.ArgsJSON, RequestedVia: approvalRequestedViaForSession(current.GetSessionKey()), ActionBinding: action.Binding, ContinuationJSON: mustMarshalJSON(continuation), Summary: action.Summary, RiskLevel: deriveApprovalRiskLevel(requested.ToolName), ExpiresAt: &expires, RequestedAt: time.Now().UTC()})
	if err != nil {
		return current, err
	}
	updated, err := s.runs.GetRun(ctx, current.GetRunId())
	if err != nil {
		return current, err
	}
	// Delivery failure does not remove the committed pending decision. It remains
	// discoverable through the approval API; a durable outbox is a separate layer.
	_ = s.config.Delivery.DeliverApprovalRequest(ctx, ApprovalRequest{ApprovalID: record.ApprovalID, RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), ToolCallID: record.ToolCallID, ToolName: record.ToolName, ArgsJSON: record.ArgsJSON, Summary: record.Summary, RiskLevel: record.RiskLevel})
	return updated, ErrApprovalSuspended
}

// After restart a transport reference cannot reconstruct authoritative history.
// Replay saved Butler context and the known result into a new provider session;
// the action itself is never re-dispatched by this helper.
func (s *Service) resumeProviderWithToolResult(ctx context.Context, current *sessionv1.RunRecord, toolCallID, resultJSON string) (transport.EventStream, error) {
	continuation, ok := ctx.Value(recoveredApprovalKey{}).(approvalContinuation)
	if !ok {
		return s.provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: current.GetRunId(), ProviderSessionRef: providerSessionRefFromRun(current), ToolCallRef: toolCallID, ToolResultJSON: resultJSON})
	}
	inputs := append([]transport.InputItem(nil), continuation.Prepared.InputItems...)
	inputs = append(inputs, transport.InputItem{Role: "system", ContentType: "application/json", Content: "Butler recovered action result (this action has already been executed; use its result and continue):\n" + mustMarshalJSON(map[string]any{"tool_call_id": toolCallID, "tool_name": continuation.ToolCall.ToolName, "args": json.RawMessage(continuation.ToolCall.ArgsJSON), "result": json.RawMessage(resultJSON)})})
	return s.provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), ProviderName: s.config.ProviderName, ModelName: s.config.ModelName, SupportsStreaming: true, SupportsToolCalls: true}, InputItems: inputs, ToolDefinitions: continuation.Prepared.ToolDefs, StreamingEnabled: true})
}

// Used by the ingress worker for denials/expiry: business-blocked does not mark
// the whole run as a technical failure and never dispatches the prepared tool.
func blockedApprovalResult(run *sessionv1.RunRecord) *ExecutionResult {
	return &ExecutionResult{RunID: run.GetRunId(), SessionKey: run.GetSessionKey(), CurrentState: commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, ReceiptStatus: "blocked"}
}
