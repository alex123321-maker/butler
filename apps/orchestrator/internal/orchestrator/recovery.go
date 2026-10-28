package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type storedCheckpoint struct {
	Interaction struct {
		Kind    string `json:"kind"`
		Version int    `json:"version"`
		ID      string `json:"interaction_id"`
	} `json:"interaction"`
	ModelRetry   modelRetryCheckpoint `json:"model_retry"`
	Prepared     preparedRun          `json:"prepared"`
	Continuation struct {
		Kind       string `json:"kind"`
		Version    int    `json:"version"`
		ApprovalID string `json:"approval_id"`
	} `json:"continuation"`
	Data struct {
		FinalMessage     string `json:"final_message"`
		AnswerState      string `json:"answer_state"`
		TerminalObserved bool   `json:"terminal_observed"`
		ToolCallID       string `json:"tool_call_id"`
	} `json:"checkpoint_data"`
}

// Resume reclaims the same Butler run from a durable checkpoint. Unknown
// external outcomes are never replayed, and provider sessions are never used
// as evidence of execution or permission.
func (s *Service) Resume(ctx context.Context, runID string, event ingress.InputEvent) (result *ExecutionResult, returnedErr error) {
	current, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if current.GetSessionKey() != event.SessionKey || s.config.Checkpoints == nil {
		return nil, fmt.Errorf("recovery identity or checkpoint unavailable")
	}
	stage, payload, err := s.config.Checkpoints.LoadCheckpoint(ctx, runID)
	if err != nil {
		return nil, err
	}
	var checkpoint storedCheckpoint
	if json.Unmarshal([]byte(payload), &checkpoint) != nil {
		return nil, fmt.Errorf("invalid durable checkpoint")
	}
	if checkpoint.ModelRetry.Reason == "authority_observation_timeout" {
		if checkpoint.ModelRetry.ExecutionDeadline.IsZero() {
			return nil, errors.New("authority_retry_deadline_required")
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, checkpoint.ModelRetry.ExecutionDeadline)
		defer cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if checkpoint.ModelRetry.Count == 1 {
		ctx = context.WithValue(ctx, modelRetryRecoveryKey{}, true)
	}
	var continuation approvalContinuation
	var decision approvals.Record
	credentialWait := checkpoint.Interaction.Kind == "credential_interaction"
	interactionWait := (checkpoint.Interaction.Kind == "communication_interaction" || credentialWait) && (stage == "awaiting_interaction" || stage == "interaction_ready")
	if interactionWait && ((!credentialWait && s.config.Communication == nil) || (credentialWait && s.config.Credentials == nil) || checkpoint.Interaction.Version != 1 || checkpoint.Interaction.ID == "") {
		return blockedApprovalResult(current, "interaction_not_resumable"), nil
	}
	if interactionWait && stage == "awaiting_interaction" {
		return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "waiting_for_user"}, nil
	}
	guardedSelection := checkpoint.Continuation.Kind == "guarded_tab_selection"
	if guardedSelection && (checkpoint.Continuation.Version != 1 || checkpoint.Continuation.ApprovalID == "" || s.config.GuardedTabRecovery == nil || stage != "approval_ready") {
		return blockedApprovalResult(current, "guarded_selection_not_resumable"), nil
	}
	if current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL && !guardedSelection && !interactionWait {
		if s.config.ApprovalService == nil {
			return nil, fmt.Errorf("approval recovery unavailable")
		}
		var raw string
		decision, raw, err = s.config.ApprovalService.LoadContinuationByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &continuation) != nil || continuation.Version != 1 || continuation.ToolCall == nil || continuation.Binding.RunID != runID {
			return nil, fmt.Errorf("unsupported approval continuation")
		}
		checkpoint.Prepared = continuation.Prepared
		if decision.Status == approvals.StatusPending {
			return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "waiting_for_user"}, nil
		}
		if decision.Status != approvals.StatusApproved {
			return blockedApprovalResult(current, "approval_"+decision.Status), nil
		}
	}
	lease, err := s.leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: event.SessionKey, RunID: runID, OwnerID: s.config.OwnerID, TTL: time.Duration(s.config.LeaseTTL) * time.Second})
	if err != nil {
		return nil, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.leases.ReleaseLease(releaseCtx, lease.LeaseID)
	}()
	durable, ok := s.leases.(interface{ Authority() *ownership.Authority })
	if !ok {
		return nil, fmt.Errorf("durable ownership required for recovery")
	}
	ctx = ownership.WithToken(ctx, durable.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	ctx = withAnswerState(ctx, checkpoint.Data.AnswerState)
	ctx = adaptiveContext(ctx, checkpoint.Prepared)
	runLog := logger.WithRunID(s.log, runID)
	retryParent := ctx
	ctx, stop, renewErrs := s.startLeaseRenewer(ctx, lease, runLog)
	defer stop()
	defer func() {
		failure := preserveOwnedCancellation(ctx, returnedErr)
		stop()
		returnedErr = s.recoverAuthorityTimeout(retryParent, ctx, current, failure)
	}()
	refresher, ok := s.runs.(interface {
		RefreshExecutionLease(context.Context, string, string) (*sessionv1.RunRecord, error)
	})
	if !ok {
		return nil, fmt.Errorf("fenced run recovery required")
	}
	current, err = refresher.RefreshExecutionLease(ctx, runID, lease.LeaseID)
	if err != nil {
		return nil, err
	}
	if stage == "communication_correction_started" && s.config.Communication != nil {
		decision, e := s.config.Communication.BeforeFinal(ctx, runID, "")
		if e != nil {
			return nil, e
		}
		if decision == "failed" {
			if blockErr := s.blockTaskRun(ctx, runID, "communication_result_missing"); blockErr != nil {
				return nil, blockErr.withPreceding(err)
			}
			return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: "communication_result_missing"}, nil
		}
		return blockedApprovalResult(current, "communication_correction_not_resumable"), nil
	}
	modelRetry := stage == "model_retry_pending" && checkpoint.ModelRetry.Count == 1 && checkpoint.ModelRetry.State == "claimed"
	if !modelRetry && !guardedSelection && !interactionWait && current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_APPROVAL && stage != "tool_result_recorded" && stage != "finalizing" && stage != "tool_dispatching" {
		if blockErr := s.blockTaskRun(ctx, runID, "checkpoint_not_resumable"); blockErr != nil {
			return nil, blockErr.withPreceding(err)
		}
		return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: "checkpoint_not_resumable"}, nil
	}
	if stage == "finalizing" && (checkpoint.Data.FinalMessage != "" || checkpoint.Data.TerminalObserved) && current.GetCurrentState() == commonv1.RunState_RUN_STATE_FINALIZING {
		if checkpoint.Data.TerminalObserved {
			markFinalSeen(ctx)
			markTerminalSeen(ctx)
		}
		return s.finalizeExecution(ctx, runLog, current, event, checkpoint.Prepared, lease.LeaseID, checkpoint.Data.FinalMessage)
	}
	if err = validateSourceCheckpoint(event, checkpoint.Prepared); err != nil {
		return nil, err
	}
	if err = s.rehydrateSourcePreparation(ctx, &checkpoint.Prepared); err != nil {
		return nil, err
	}
	ctx = converterContinuationProposalContext(ctx, checkpoint.Prepared)
	if continuation.Prepared.SourceInputEvent != nil || checkpoint.Prepared.DelegatedTaskID != "" {
		continuation.Prepared = checkpoint.Prepared
	}
	var stream transport.EventStream
	if modelRetry {
		stream, err = s.resumeModelRetry(ctx, current, checkpoint.Prepared, checkpoint.ModelRetry)
		if err == nil && current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
		}
	} else if interactionWait {
		if stage != "interaction_ready" {
			return blockedApprovalResult(current, "interaction_not_ready"), nil
		}
		var answer string
		var resumeErr error
		if credentialWait {
			answer, resumeErr = s.config.Credentials.ResumeInteraction(ctx, runID, checkpoint.Interaction.ID)
		} else {
			answer, resumeErr = s.config.Communication.ResumeInteraction(ctx, runID, checkpoint.Interaction.ID)
		}
		if resumeErr != nil {
			if errors.Is(resumeErr, context.Canceled) || errors.Is(resumeErr, context.DeadlineExceeded) {
				return nil, resumeErr
			}
			code := "interaction_invalidated"
			var diagnostic interface{ InteractionResumeCode() string }
			if errors.As(resumeErr, &diagnostic) {
				code = diagnostic.InteractionResumeCode()
			}
			return blockedApprovalResult(current, code), nil
		}
		refreshed, e := s.refreshRecoveredToolContext(ctx, runID, checkpoint.Prepared.InputItems, checkpoint.Prepared.ToolDefs)
		if e != nil {
			return nil, e
		}
		checkpoint.Prepared = interactionResponsePreparation(checkpoint.Prepared, refreshed, checkpoint.Interaction.ID, answer)
		if !credentialWait {
			// ResumeInteraction has verified the canonical owning run/Input and
			// interaction. Upgrade legacy checkpoints without accepting payload refs.
			if checkpoint.Prepared.CurrentRequest == nil && isCurrentOwnerRequest(event, checkpoint.Prepared) {
				checkpoint.Prepared.CurrentRequest = &currentRequestContext{RunRef: runID, InputEventRef: "input:" + event.EventID}
			}
			attachCurrentRequestContext(&checkpoint.Prepared, checkpoint.Interaction.ID)
		}
		retainer, ok := s.config.Checkpoints.(interface {
			SaveInteractionPreparation(context.Context, string, string, string) error
		})
		if !ok {
			return nil, fmt.Errorf("durable interaction preparation unavailable")
		}
		if e = retainer.SaveInteractionPreparation(ctx, runID, checkpoint.Interaction.ID, mustMarshalJSON(durableSourcePreparation(durableSkillPreparation(checkpoint.Prepared)))); e != nil {
			return nil, e
		}
		items := append([]transport.InputItem(nil), checkpoint.Prepared.InputItems...)
		history, e := s.transcript.GetTranscript(ctx, event.SessionKey)
		if e != nil {
			return nil, e
		}
		for _, call := range history.ToolCalls {
			if call.RunID == runID && call.FinishedAt != nil {
				if call.Status == "effect_unknown" {
					return uncertainResult(current), nil
				}
				items = append(items, recoveredActionResultItem(call.ToolCallID, call.ToolName, call.ArgsJSON, recordedToolResultEnvelope(call)))
			}
		}
		if current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, lease.LeaseID, "", "")
			if err != nil {
				return nil, err
			}
		}
		if current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME && current.GetCurrentState() != commonv1.RunState_RUN_STATE_MODEL_RUNNING {
			return blockedApprovalResult(current, "interaction_phase_changed"), nil
		}
		if err = s.discardProviderContext(ctx, runID); err != nil {
			return nil, err
		}
		stream, err = s.provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: runID, SessionKey: event.SessionKey, ProviderName: s.config.ProviderName, ModelName: s.config.ModelName, SupportsStreaming: true, SupportsToolCalls: true}, InputItems: items, ToolDefinitions: refreshed.ToolDefs, StreamingEnabled: true})
		if err == nil && current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
		}
	} else if guardedSelection {
		// Reload capability DATA before task activation. Catalog changes during
		// reload are then checked by the atomic owning selection/source guard.
		refreshed, refreshErr := s.refreshRecoveredToolContext(ctx, runID, checkpoint.Prepared.InputItems, checkpoint.Prepared.ToolDefs)
		if refreshErr != nil {
			return nil, refreshErr
		}
		sessionID, resumeErr := s.config.GuardedTabRecovery.ResumeSelection(ctx, runID, checkpoint.Continuation.ApprovalID)
		if resumeErr != nil {
			return blockedApprovalResult(current, "guarded_selection_invalidated"), nil
		}
		inputs := append([]transport.InputItem(nil), refreshed.InputItems...)
		inputs = append(inputs, content.Data(map[string]any{"kind": "guarded_tab_selection", "single_tab_session_id": sessionID}, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{"approval:" + checkpoint.Continuation.ApprovalID}, Sensitivity: content.Private}))
		if current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, lease.LeaseID, "", "")
			if err != nil {
				return nil, err
			}
		}
		if current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME && current.GetCurrentState() != commonv1.RunState_RUN_STATE_MODEL_RUNNING {
			return blockedApprovalResult(current, "guarded_selection_not_resumable"), nil
		}
		if err = s.discardProviderContext(ctx, runID); err != nil {
			return nil, err
		}
		providerCtx, release, beginErr := ownership.Begin(ctx)
		if beginErr != nil {
			return nil, beginErr
		}
		stream, err = s.provider.StartRun(providerCtx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: runID, SessionKey: event.SessionKey, ProviderName: s.config.ProviderName, ModelName: s.config.ModelName, SupportsStreaming: true, SupportsToolCalls: true}, InputItems: inputs, ToolDefinitions: refreshed.ToolDefs, StreamingEnabled: true})
		release()
		if err == nil && current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
		}
	} else if current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
		if _, err = s.config.ApprovalService.VerifyActionApproval(ctx, actionauth.ApprovalRequest{ApprovalID: decision.ApprovalID, Binding: continuation.Binding}); err != nil {
			if blockErr := s.blockTaskRun(ctx, runID, "approval_invalidated"); blockErr != nil {
				return nil, blockErr.withPreceding(err)
			}
			return blockedApprovalResult(current, "approval_invalidated"), nil
		}
		ctx = context.WithValue(ctx, recoveredApprovalKey{}, continuation)
		stream, current, err = s.resumeApprovedToolCall(ctx, runLog, current, lease.LeaseID, continuation.ToolCall, decision.ApprovalID)
	} else {
		history, readErr := s.transcript.GetTranscript(ctx, event.SessionKey)
		if readErr != nil {
			return nil, readErr
		}
		var result *toolbrokerv1.ToolResult
		var resultJSON string
		for _, call := range history.ToolCalls {
			if call.RunID == runID && call.ToolCallID == checkpoint.Data.ToolCallID && call.FinishedAt != nil && call.ResultJSON != "" {
				continuation = approvalContinuation{Version: 1, Prepared: checkpoint.Prepared, ToolCall: &transport.ToolCallRequest{ToolCallRef: call.ToolCallID, ToolName: call.ToolName, ArgsJSON: call.ArgsJSON}}
				result = &toolbrokerv1.ToolResult{RunId: runID, ToolCallId: call.ToolCallID, ToolName: call.ToolName, Status: call.Status, ResultJson: call.ResultJSON}
				resultJSON = recordedToolResultEnvelope(call)
			}
		}
		if result == nil || result.GetStatus() == "effect_unknown" {
			if blockErr := s.blockTaskRun(ctx, runID, "effect_unknown"); blockErr != nil {
				return nil, blockErr.withPreceding(err)
			}
			return uncertainResult(current), nil
		}
		ctx = context.WithValue(ctx, recoveredApprovalKey{}, continuation)
		if s.config.ApprovalService != nil {
			if err = s.config.ApprovalService.MarkActionCheckpoint(ctx, runID, result.GetToolCallId(), "completed"); err != nil {
				return nil, err
			}
		}
		if current.GetCurrentState() == commonv1.RunState_RUN_STATE_TOOL_RUNNING {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, lease.LeaseID, "", "")
			if err != nil {
				return nil, err
			}
		}
		if current.GetCurrentState() != commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME && current.GetCurrentState() != commonv1.RunState_RUN_STATE_MODEL_RUNNING {
			return uncertainResult(current), nil
		}
		stream, err = s.resumeProviderWithToolResult(ctx, current, result.GetToolCallId(), resultJSON)
		if err == nil && current.GetCurrentState() == commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME {
			current, err = s.transition(ctx, runID, current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
		}
	}
	if err != nil {
		if errors.Is(err, ErrApprovalSuspended) {
			return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "waiting_for_user"}, nil
		}
		if errors.Is(err, ErrCommunicationStatePending) {
			return nil, err
		}
		if s.scheduleModelRetry(ctx, current, err) {
			return nil, ErrModelRetryPending
		}
		if _, stopped := budgetStopReason(err); stopped {
			return s.stopBudget(ctx, current, err)
		}
		if errors.Is(err, ErrActionOutcomeUnknown) {
			if blockErr := s.blockTaskRun(ctx, runID, "effect_unknown"); blockErr != nil {
				return nil, blockErr.withPreceding(err)
			}
			return uncertainResult(current), nil
		}
		if blockErr := s.blockTaskRun(ctx, runID, "continuation_authorization_or_transport_failed"); blockErr != nil {
			return nil, blockErr.withPreceding(err)
		}
		return nil, err
	}
	final, current, err := s.consumeModelStream(ctx, runLog, current, event.SessionKey, lease.LeaseID, stream)
	if errors.Is(err, ErrCommunicationStatePending) {
		return nil, err
	}
	if err != nil && s.scheduleModelRetry(ctx, current, err) {
		return nil, ErrModelRetryPending
	}
	if _, stopped := budgetStopReason(err); stopped {
		return s.stopBudget(ctx, current, err)
	}
	if errors.Is(err, ErrApprovalSuspended) {
		return &ExecutionResult{RunID: runID, SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "waiting_for_user"}, nil
	}
	if errors.Is(err, ErrActionOutcomeUnknown) {
		if blockErr := s.blockTaskRun(ctx, runID, "effect_unknown"); blockErr != nil {
			return nil, blockErr.withPreceding(err)
		}
		return uncertainResult(current), nil
	}
	if err != nil {
		return nil, err
	}
	if err = drainRenewError(renewErrs); err != nil {
		return nil, err
	}
	return s.finalizeExecution(ctx, runLog, current, event, checkpoint.Prepared, lease.LeaseID, final)
}
func uncertainResult(run *sessionv1.RunRecord) *ExecutionResult {
	return &ExecutionResult{RunID: run.GetRunId(), SessionKey: run.GetSessionKey(), CurrentState: run.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: "effect_unknown"}
}
