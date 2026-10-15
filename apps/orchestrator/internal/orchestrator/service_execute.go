package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) Execute(ctx context.Context, event ingress.InputEvent) (*ExecutionResult, error) {
	if err := validateInputEvent(event); err != nil {
		return nil, err
	}
	// Identity is needed for durable acceptance; model context is built only
	// after exclusive session ownership has been obtained.
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return nil, err
	}
	userID := firstString(payload, "user_id", "external_user_id", "author_id")
	if userID == "" {
		userID = event.SessionKey
	}

	if _, _, err := s.sessions.CreateSession(ctx, session.CreateSessionParams{
		SessionKey:   event.SessionKey,
		UserID:       userID,
		Channel:      event.Source,
		MetadataJSON: mustMarshalJSON(map[string]any{"source": event.Source}),
	}); err != nil {
		return nil, fmt.Errorf("ensure session: %w", err)
	}

	runRecord, err := s.runs.CreateRun(ctx, &sessionv1.CreateRunRequest{
		SessionKey:    event.SessionKey,
		InputEvent:    event.ToProto(),
		AutonomyMode:  s.config.AutonomyMode,
		ModelProvider: s.config.ProviderName,
		MetadataJson: mustMarshalJSON(map[string]any{
			"input_payload": payload,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	runLog := logger.WithRunID(s.log, runRecord.GetRunId())
	if err := s.checkpoint(ctx, runRecord.GetRunId(), "run_created", map[string]any{}); err != nil {
		return nil, err
	}
	return s.executeRun(ctx, runLog, runRecord, event, preparedRun{})
}

func (s *Service) executeRun(ctx context.Context, runLog *slog.Logger, runRecord *sessionv1.RunRecord, event ingress.InputEvent, prepared preparedRun) (*ExecutionResult, error) {
	current := runRecord

	var next *sessionv1.RunRecord
	var err error
	if current.GetCurrentState() == commonv1.RunState_RUN_STATE_CREATED {
		next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_QUEUED, "", "", "")
		if err != nil {
			return nil, s.failRun(ctx, current, "", runLog, err)
		}
		current = next
	}

	lease, err := s.leases.AcquireLease(ctx, session.AcquireLeaseParams{
		SessionKey: event.SessionKey,
		RunID:      current.GetRunId(),
		OwnerID:    s.config.OwnerID,
		TTL:        time.Duration(s.config.LeaseTTL) * time.Second,
	})
	if err != nil {
		if !errors.Is(err, session.ErrLeaseConflict) {
			return nil, s.failRun(ctx, current, "", runLog, err)
		}
		return nil, fmt.Errorf("acquire lease: %w", err)
	}
	if durable, ok := s.leases.(interface{ Authority() *ownership.Authority }); ok {
		ctx = ownership.WithToken(ctx, durable.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	}
	runCtx, stopRenewing, renewErrs := s.startLeaseRenewer(ctx, lease, runLog)
	ctx = runCtx
	defer func() {
		stopRenewing()
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		if _, releaseErr := s.leases.ReleaseLease(releaseCtx, lease.LeaseID); releaseErr != nil {
			runLog.Error("release lease failed", slog.String("lease_id", lease.LeaseID), slog.String("error", releaseErr.Error()))
		}
	}()
	if current.GetCurrentState() == commonv1.RunState_RUN_STATE_ACQUIRED || current.GetCurrentState() == commonv1.RunState_RUN_STATE_PREPARING {
		recoverer, ok := s.runs.(interface {
			RecoverPreparation(context.Context, string, string) (*sessionv1.RunRecord, error)
		})
		if !ok {
			return nil, fmt.Errorf("safe checkpoint recovery unavailable")
		}
		current, err = recoverer.RecoverPreparation(ctx, current.GetRunId(), lease.LeaseID)
		if err != nil {
			return nil, err
		}
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_ACQUIRED, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run acquired: %w", err))
	}
	current = next

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_PREPARING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run preparing: %w", err))
	}
	current = next

	opaqueEvent := event
	event, err = s.prepareSourceInput(ctx, event)
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}
	prepared, err = s.prepareRun(ctx, event)
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}
	if opaqueSourceInput(opaqueEvent) {
		prepared.SourceInputEvent = &opaqueEvent
	}
	if writer, ok := s.runs.(interface {
		PersistPreparation(context.Context, string, string) error
	}); ok {
		durablePrepared := durableSourcePreparation(prepared)
		if err = writer.PersistPreparation(ctx, current.GetRunId(), mustMarshalJSON(map[string]any{"input_payload": durablePrepared.InputPayload, "memory_bundle": durablePrepared.MemoryBundle})); err != nil {
			return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
		}
	}
	for _, item := range []struct {
		kind    string
		payload map[string]any
	}{{observability.EventMemoryLoaded, prepared.observabilityMemory}, {observability.EventToolsLoaded, prepared.observabilityTools}, {observability.EventPromptAssembled, prepared.observabilityPrompt}} {
		if item.payload != nil {
			s.emitEvent(ctx, current.GetRunId(), event.SessionKey, item.kind, item.payload)
		}
	}
	if err := s.checkpoint(ctx, current.GetRunId(), "prepared", prepared); err != nil {
		return nil, err
	}

	if statusErr := s.config.Delivery.DeliverStatusEvent(ctx, StatusEvent{
		RunID:      current.GetRunId(),
		SessionKey: event.SessionKey,
		Status:     "thinking",
	}); statusErr != nil {
		runLog.Warn("failed to deliver thinking status", slog.String("error", statusErr.Error()))
	}

	if !prepared.InputAlreadyRecorded {
		if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
			SessionKey:   event.SessionKey,
			RunID:        current.GetRunId(),
			Role:         "user",
			Content:      prepared.UserMessage,
			MetadataJSON: mustMarshalJSON(map[string]any{"event_id": event.EventID, "source": event.Source}),
		}); err != nil {
			return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append user transcript: %w", err))
		}
	}

	if err := s.savePreparedWorkingMemory(ctx, current.GetRunId(), event.SessionKey, prepared); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save working memory: %w", err))
	}
	if err := s.saveTransientWorkingState(ctx, current.GetRunId(), event.SessionKey, "preparing", map[string]any{"goal": prepared.WorkingMemory.Goal}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save transient working memory: %w", err))
	}
	// New runs replay Butler-owned context into a fresh provider session. There
	// is no durable provider replay boundary yet, so cross-run reuse can duplicate
	// messages. Tool-result continuation within this run still uses its own ref.

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run model_running: %w", err))
	}
	current = next
	if err := s.checkpoint(ctx, current.GetRunId(), "model_started", prepared); err != nil {
		return nil, err
	}

	existingProviderSessionRef, err := transport.ParseProviderSessionRef(current.GetProviderSessionRef())
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("parse persisted provider session ref: %w", err))
	}

	transportCtx := transport.WithSessionKey(runCtx, event.SessionKey)
	transportCtx = converterContinuationProposalContext(transportCtx, prepared)
	transportCtx = withAnswerState(transportCtx, "final")
	var stream transport.EventStream
	if operatorDoctor(ctx) {
		stream = doctorCommandStream(current.GetRunId())
	} else {
		stream, err = s.provider.StartRun(transportCtx, transport.StartRunRequest{
			Context: transport.TransportRunContext{
				RunID:                  current.GetRunId(),
				SessionKey:             event.SessionKey,
				ProviderName:           s.config.ProviderName,
				ModelName:              s.config.ModelName,
				ProviderSessionRef:     existingProviderSessionRef,
				SupportsStreaming:      true,
				SupportsToolCalls:      true,
				SupportsStatefulResume: true,
			},
			InputItems:       prepared.InputItems,
			ToolDefinitions:  prepared.ToolDefs,
			StreamingEnabled: true,
		})
	}
	if err != nil {
		if s.scheduleModelRetry(transportCtx, current, err) {
			return nil, ErrModelRetryPending
		}
		if _, stopped := budgetStopReason(err); stopped {
			return s.stopBudget(ctx, current, err)
		}
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}

	finalMessage, current, err := s.consumeModelStream(transportCtx, runLog, current, event.SessionKey, lease.LeaseID, stream)
	if err != nil {
		if s.scheduleModelRetry(transportCtx, current, err) {
			return nil, ErrModelRetryPending
		}
		if _, stopped := budgetStopReason(err); stopped {
			return s.stopBudget(ctx, current, err)
		}
		if errors.Is(err, ErrApprovalSuspended) {
			result := &ExecutionResult{RunID: current.GetRunId(), SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "waiting_for_user"}
			if prepared.Task != nil {
				result.TaskID = prepared.Task.TaskID
			}
			return result, nil
		}
		if errors.Is(err, ErrActionOutcomeUnknown) {
			s.blockTaskRun(ctx, current.GetRunId(), "effect_unknown")
			return &ExecutionResult{RunID: current.GetRunId(), SessionKey: event.SessionKey, CurrentState: current.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: "effect_unknown"}, nil
		}
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}
	if renewErr := drainRenewError(renewErrs); renewErr != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, renewErr)
	}
	return s.finalizeExecution(transportCtx, runLog, current, event, prepared, lease.LeaseID, finalMessage)
}

func (s *Service) blockTaskRun(ctx context.Context, runID, reason string) {
	if tasks, ok := s.config.Tasks.(interface {
		BlockRun(context.Context, string, string) error
	}); ok {
		_ = tasks.BlockRun(ctx, runID, reason)
	}
}

func (s *Service) finalizeExecution(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, event ingress.InputEvent, prepared preparedRun, leaseID, finalMessage string) (*ExecutionResult, error) {
	var next *sessionv1.RunRecord
	var err error
	if strings.TrimSpace(finalMessage) == "" {
		return nil, s.failRun(ctx, current, leaseID, runLog, fmt.Errorf("model run completed without assistant_final event"))
	}

	if current.GetCurrentState() != commonv1.RunState_RUN_STATE_FINALIZING {
		next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_FINALIZING, leaseID, "", "")
		if err != nil {
			return nil, s.failRun(ctx, current, leaseID, runLog, fmt.Errorf("mark run finalizing: %w", err))
		}
		current = next
	}
	if err := s.checkpoint(ctx, current.GetRunId(), "finalizing", map[string]any{"final_message": finalMessage, "answer_state": answerState(ctx)}); err != nil {
		return nil, err
	}
	if s.config.Completion == nil {
		return nil, fmt.Errorf("atomic result persistence unavailable")
	}
	request := completion.Request{RunID: current.GetRunId(), SessionKey: event.SessionKey, Content: finalMessage, AnswerState: answerState(ctx), PipelineEnabled: s.config.DurableMemoryPipeline}
	if prepared.Task != nil {
		request.TaskID = prepared.Task.TaskID
		request.TaskRevision = prepared.Task.Revision
		request.ConstraintsRevision = prepared.Task.ConstraintsRevision
	}
	saved, err := s.config.Completion.Save(ctx, request)
	// Keep the finalizing checkpoint on persistence failure. Recovery may save
	// this answer again; it must not call the model or replay external effects.
	if err != nil {
		if errors.Is(err, ownership.ErrStale) || errors.Is(err, task.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrResultPersistencePending, err)
	}
	runLog.Info("run completed", slog.String("session_key", event.SessionKey))
	s.emitEvent(ctx, current.GetRunId(), event.SessionKey, observability.EventAssistantFinal, map[string]any{"result_id": saved.ResultID, "answer_state": saved.AnswerState, "verdict": saved.Verdict, "content_length": len(finalMessage)})
	s.emitEvent(ctx, current.GetRunId(), event.SessionKey, observability.EventRunCompleted, map[string]any{"result_id": saved.ResultID, "delivery": "stored_outbox"})
	s.scheduleHubCleanup(current.GetRunId())
	result := &ExecutionResult{RunID: current.GetRunId(), SessionKey: event.SessionKey, CurrentState: commonv1.RunState_RUN_STATE_COMPLETED, AssistantResponse: finalMessage, ResultID: saved.ResultID, AnswerState: saved.AnswerState, Verdict: saved.Verdict}
	if prepared.Task != nil {
		result.TaskID = prepared.Task.TaskID
	}
	return result, nil
}

func (s *Service) consumeModelStream(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, initialStream transport.EventStream) (string, *sessionv1.RunRecord, error) {
	var finalMessage string
	stream := initialStream
	partialDelivery := true

	for stream != nil {
		var nextStream transport.EventStream
	eventLoop:
		for {
			var transportEvent transport.TransportEvent
			select {
			case <-ctx.Done():
				return "", current, ctx.Err()
			case event, ok := <-stream:
				if !ok {
					break eventLoop
				}
				transportEvent = event
			}
			var err error
			if current, err = s.persistProviderSessionRef(ctx, current, transportEvent.ProviderSessionRef, runLog); err != nil {
				return "", current, err
			}
			switch transportEvent.EventType {
			case transport.EventTypeAssistantDelta:
				if transportEvent.AssistantDelta != nil {
					if partialDelivery {
						if err := s.config.Delivery.DeliverAssistantDelta(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: transportEvent.AssistantDelta.Content, SequenceNo: transportEvent.AssistantDelta.SequenceNo}); err != nil {
							partialDelivery = false
							runLog.Warn("partial channel unavailable; work continues")
						}
					}
					s.emitEvent(ctx, current.GetRunId(), sessionKey, observability.EventAssistantDelta, map[string]any{"sequence_no": transportEvent.AssistantDelta.SequenceNo, "content_length": len(transportEvent.AssistantDelta.Content)})
				}
			case transport.EventTypeAssistantFinal:
				if transportEvent.AssistantFinal != nil {
					finalMessage = transportEvent.AssistantFinal.Content
					resetAnswerState(ctx)
					if partialFinishReason(transportEvent.AssistantFinal.FinishReason) {
						setPartialAnswer(ctx)
					}
				}
			case transport.EventTypeToolCallRequested:
				// A tool proposal supersedes any earlier apparent answer, even if
				// the continuation returns a nil stream. Only a new final may commit.
				finalMessage = ""
				resetAnswerState(ctx)
				resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, transportEvent.ToolCall)
				if err != nil {
					return "", updated, err
				}
				current = updated
				nextStream = resumed
				break eventLoop
			case transport.EventTypeToolCallBatchRequested:
				resumedFinal, updated, err := s.handleToolBatch(ctx, runLog, current, sessionKey, leaseID, transportEvent.ToolCallBatch)
				if err != nil {
					return "", updated, err
				}
				current = updated
				if strings.TrimSpace(resumedFinal) != "" {
					finalMessage = resumedFinal
				}
			case transport.EventTypeTransportError:
				if transportEvent.TransportError != nil && transportEvent.TransportError.Type == "model_turn_timeout" {
					return "", current, s.modelTimeoutFromCheckpoint(ctx, current.GetRunId())
				}
				if transportEvent.TransportError != nil && (transportEvent.TransportError.Type == skillInvalidReason || transportEvent.TransportError.Type == "execution_budget_deadline" || transportEvent.TransportError.Type == "execution_budget_tokens" || transportEvent.TransportError.Type == transport.ErrorTypeInputBytesExceeded) {
					return "", current, &budget.LoopError{Reason: string(transportEvent.TransportError.Type)}
				}
				return "", current, transportError(transportEvent.TransportError)
			case transport.EventTypeRunTimedOut:
				return "", current, &transport.Error{Type: transport.ErrorTypeProviderTimeout, Message: "provider_timeout"}
			case transport.EventTypeRunCancelled:
				return "", current, context.Canceled
			case transport.EventTypeTransportWarning:
				runLog.Warn("transport warning", slog.String("payload", transportEvent.PayloadJSON))
			}
		}
		stream = nextStream
	}
	return finalMessage, current, nil
}

func (s *Service) failRun(ctx context.Context, current *sessionv1.RunRecord, leaseID string, runLog *slog.Logger, err error) error {
	// Typed provider errors may contain untrusted details even after wrapping.
	// Persist only the fixed subtype while retaining the general run error class.
	if code := transport.DiagnosticCode(err); code != "" {
		err = &transport.Error{Type: transport.ErrorType(code), Message: code}
	}
	runLog.Error("run execution failed", slog.String("error", err.Error()))
	s.emitEvent(ctx, current.GetRunId(), current.GetSessionKey(), observability.EventRunError, map[string]any{
		"error_message": truncateForObservability(err.Error(), 500),
		"error_type":    classifyExecutionError(err),
	})
	state := current.GetCurrentState()
	if current != nil {
		if finalizeErr := s.finalizeWorkingMemory(ctx, current.GetSessionKey(), current.GetRunId(), commonv1.RunState_RUN_STATE_FAILED, err.Error()); finalizeErr != nil {
			runLog.Warn("working memory finalization failed on error", slog.String("error", finalizeErr.Error()))
		}
		if finalizeErr := s.finalizeTransientWorkingState(ctx, current.GetSessionKey(), current.GetRunId(), commonv1.RunState_RUN_STATE_FAILED, err.Error()); finalizeErr != nil {
			runLog.Warn("transient working memory finalization failed on error", slog.String("error", finalizeErr.Error()))
		}
	}
	if state == commonv1.RunState_RUN_STATE_UNSPECIFIED || state == commonv1.RunState_RUN_STATE_FAILED || state == commonv1.RunState_RUN_STATE_CANCELLED || state == commonv1.RunState_RUN_STATE_TIMED_OUT || state == commonv1.RunState_RUN_STATE_COMPLETED {
		return err
	}
	if _, transitionErr := s.transition(ctx, current.GetRunId(), state, commonv1.RunState_RUN_STATE_FAILED, leaseID, classifyExecutionError(err), err.Error()); transitionErr != nil {
		return fmt.Errorf("%w: additionally failed to transition run to failed: %v", err, transitionErr)
	}
	s.scheduleHubCleanup(current.GetRunId())
	return err
}
