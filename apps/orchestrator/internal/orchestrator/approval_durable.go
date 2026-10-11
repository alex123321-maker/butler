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
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

var ErrApprovalSuspended = errors.New("execution suspended for durable owner decision")

type approvalContinuation struct {
	Version          int                        `json:"version"`
	Prepared         preparedRun                `json:"prepared"`
	ToolCall         *transport.ToolCallRequest `json:"tool_call"`
	Binding          actionauth.ActionBinding   `json:"binding"`
	LogicalActionRef string                     `json:"logical_action_ref,omitempty"`
	EffectID         string                     `json:"effect_id,omitempty"`
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
	var wrapper struct {
		Prepared json.RawMessage `json:"prepared"`
	}
	if json.Unmarshal([]byte(payload), &wrapper) == nil && len(wrapper.Prepared) > 0 {
		payload = string(wrapper.Prepared)
	}
	if err = json.Unmarshal([]byte(payload), &prepared); err != nil {
		return current, err
	}
	continuation := approvalContinuation{Version: 1, Prepared: durableSourcePreparation(durableSkillPreparation(prepared)), ToolCall: requested, Binding: action.Binding, LogicalActionRef: actionauth.LogicalReference(ctx), EffectID: action.EffectID}
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
	// The durable continuation contains Butler data only. Drop private provider
	// call/signature state before releasing session ownership, including when the
	// owner decides immediately and recovery happens in this same process.
	if err := s.discardProviderContext(ctx, current.GetRunId()); err != nil {
		s.log.Warn("provider context cleanup after durable suspension failed")
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
	if !ok || continuation.ToolCall == nil || continuation.ToolCall.ToolCallRef != toolCallID {
		return s.provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: current.GetRunId(), ProviderSessionRef: providerSessionRefFromRun(current), ToolCallRef: toolCallID, ToolResultJSON: resultJSON})
	}
	inputs := append([]transport.InputItem(nil), continuation.Prepared.InputItems...)
	// Saved package data and definitions describe a past catalog. Reload current
	// task-scoped capabilities before a fresh provider attempt, never resurrect a
	// disabled/upgraded extension through a durable continuation.
	refreshed, err := s.refreshRecoveredToolContext(ctx, current.GetRunId(), inputs, continuation.Prepared.ToolDefs)
	if err != nil {
		return nil, err
	}
	inputs = refreshed.InputItems
	history, err := s.transcript.GetTranscript(ctx, current.GetSessionKey())
	if err != nil {
		return nil, fmt.Errorf("load recorded action history: %w", err)
	}
	currentRecorded := false
	for _, call := range history.ToolCalls {
		if call.RunID != current.GetRunId() || call.FinishedAt == nil {
			continue
		}
		if call.Status == "effect_unknown" {
			return nil, ErrActionOutcomeUnknown
		}
		knownResult := recordedToolResultEnvelope(call)
		if call.ToolCallID == toolCallID {
			currentRecorded = true
			knownResult = resultJSON
		}
		inputs = append(inputs, recoveredActionResultItem(call.ToolCallID, call.ToolName, call.ArgsJSON, knownResult))
	}
	if !currentRecorded {
		inputs = append(inputs, recoveredActionResultItem(toolCallID, continuation.ToolCall.ToolName, continuation.ToolCall.ArgsJSON, resultJSON))
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	// A stopped worker may leave a pending context in the same process. Its
	// opaque provider reference is neither required nor reused for recovery.
	if err := s.discardProviderContext(ctx, current.GetRunId()); err != nil {
		return nil, err
	}
	return s.provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), ProviderName: s.config.ProviderName, ModelName: s.config.ModelName, SupportsStreaming: true, SupportsToolCalls: true}, InputItems: inputs, ToolDefinitions: refreshed.ToolDefs, StreamingEnabled: true})
}

func (s *Service) discardProviderContext(ctx context.Context, runID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = s.provider.CancelRun(ctx, transport.CancelRunRequest{RunID: runID, Reason: "durable_context_recovery"})
	return err
}

func recordedToolResultEnvelope(call transcript.ToolCall) string {
	return mustMarshalJSON(map[string]any{"status": call.Status, "result": json.RawMessage(normalizeJSON(call.ResultJSON, "{}")), "error": json.RawMessage(normalizeJSON(call.ErrorJSON, "{}"))})
}

func recoveredActionResultItem(toolCallID, toolName, argsJSON, resultJSON string) transport.InputItem {
	// Observations are data, not system instructions or native functionResponse
	// parts. The matching provider functionCall/signature is intentionally absent
	// after a restart; only live continuation may use those private transport parts.
	return content.Data(map[string]any{"already_executed": true, "tool_call_id": toolCallID, "tool_name": toolName, "args": json.RawMessage(normalizeJSON(argsJSON, "{}")), "result": json.RawMessage(normalizeJSON(resultJSON, "{}"))}, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{"tool_call:" + toolCallID}, Sensitivity: content.Private})
}

// Used by the ingress worker for denials/expiry: business-blocked does not mark
// the whole run as a technical failure and never dispatches the prepared tool.
func blockedApprovalResult(run *sessionv1.RunRecord, reason string) *ExecutionResult {
	return &ExecutionResult{RunID: run.GetRunId(), SessionKey: run.GetSessionKey(), CurrentState: commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, ReceiptStatus: "blocked", ErrorCode: reason}
}
