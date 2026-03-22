package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func (s *Service) Execute(ctx context.Context, event ingress.InputEvent) (*ExecutionResult, error) {
	if err := validateInputEvent(event); err != nil {
		return nil, err
	}
	prepared, err := s.prepareRun(ctx, event)
	if err != nil {
		return nil, err
	}

	if _, _, err := s.sessions.CreateSession(ctx, session.CreateSessionParams{
		SessionKey:   event.SessionKey,
		UserID:       prepared.SessionUserID,
		Channel:      prepared.Channel,
		MetadataJSON: mustMarshalJSON(map[string]any{"source": event.Source}),
	}); err != nil {
		return nil, fmt.Errorf("ensure session: %w", err)
	}

	runRecord, err := s.runs.CreateRun(ctx, &sessionv1.CreateRunRequest{
		SessionKey:    event.SessionKey,
		InputEvent:    event.ToProto(),
		AutonomyMode:  commonv1.AutonomyMode_AUTONOMY_MODE_1,
		ModelProvider: s.config.ProviderName,
		MetadataJson: mustMarshalJSON(map[string]any{
			"input_payload": prepared.InputPayload,
			"memory_bundle": prepared.MemoryBundle,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	runLog := logger.WithRunID(s.log, runRecord.GetRunId())
	if prepared.observabilityMemory != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventMemoryLoaded, prepared.observabilityMemory)
	}
	if prepared.observabilityTools != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventToolsLoaded, prepared.observabilityTools)
	}
	if prepared.observabilityPrompt != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventPromptAssembled, prepared.observabilityPrompt)
	}

	return s.executeRun(ctx, runLog, runRecord, event, prepared)
}

func (s *Service) executeRun(ctx context.Context, runLog *slog.Logger, runRecord *sessionv1.RunRecord, event ingress.InputEvent, prepared preparedRun) (*ExecutionResult, error) {
	current := runRecord

	next, err := s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_QUEUED, "", "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, "", runLog, fmt.Errorf("queue run: %w", err))
	}
	current = next

	lease, err := s.leases.AcquireLease(ctx, session.AcquireLeaseParams{
		SessionKey: event.SessionKey,
		RunID:      current.GetRunId(),
		OwnerID:    s.config.OwnerID,
		TTL:        time.Duration(s.config.LeaseTTL) * time.Second,
	})
	if err != nil {
		return nil, s.failRun(ctx, current, "", runLog, fmt.Errorf("acquire lease: %w", err))
	}
	runCtx, stopRenewing, renewErrs := s.startLeaseRenewer(ctx, lease, runLog)
	defer stopRenewing()
	defer func() {
		if _, releaseErr := s.leases.ReleaseLease(context.Background(), lease.LeaseID); releaseErr != nil {
			runLog.Error("release lease failed", slog.String("lease_id", lease.LeaseID), slog.String("error", releaseErr.Error()))
		}
	}()

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

	if statusErr := s.config.Delivery.DeliverStatusEvent(ctx, StatusEvent{
		RunID:      current.GetRunId(),
		SessionKey: event.SessionKey,
		Status:     "thinking",
	}); statusErr != nil {
		runLog.Warn("failed to deliver thinking status", slog.String("error", statusErr.Error()))
	}

	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
		SessionKey:   event.SessionKey,
		RunID:        current.GetRunId(),
		Role:         "user",
		Content:      prepared.UserMessage,
		MetadataJSON: mustMarshalJSON(map[string]any{"event_id": event.EventID, "source": event.Source}),
	}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append user transcript: %w", err))
	}

	if err := s.savePreparedWorkingMemory(ctx, current.GetRunId(), event.SessionKey, prepared); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save working memory: %w", err))
	}
	if err := s.saveTransientWorkingState(ctx, current.GetRunId(), event.SessionKey, "preparing", map[string]any{"goal": prepared.WorkingMemory.Goal}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save transient working memory: %w", err))
	}
	current = s.attachReusableProviderSessionRef(ctx, current, runLog)

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run model_running: %w", err))
	}
	current = next

	existingProviderSessionRef, err := transport.ParseProviderSessionRef(current.GetProviderSessionRef())
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("parse persisted provider session ref: %w", err))
	}

	transportCtx := transport.WithSessionKey(runCtx, event.SessionKey)
	stream, err := s.provider.StartRun(transportCtx, transport.StartRunRequest{
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
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}

	finalMessage, current, err := s.consumeModelStream(transportCtx, runLog, current, event.SessionKey, lease.LeaseID, stream)
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}
	if renewErr := drainRenewError(renewErrs); renewErr != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, renewErr)
	}
	if strings.TrimSpace(finalMessage) == "" {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("model run completed without assistant_final event"))
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_FINALIZING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run finalizing: %w", err))
	}
	current = next

	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
		SessionKey:   event.SessionKey,
		RunID:        current.GetRunId(),
		Role:         "assistant",
		Content:      finalMessage,
		MetadataJSON: `{"source":"model"}`,
	}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append assistant transcript: %w", err))
	}
	if s.config.Artifacts != nil {
		if _, saveErr := s.config.Artifacts.SaveAssistantFinal(ctx, current.GetRunId(), event.SessionKey, finalMessage, time.Now().UTC()); saveErr != nil {
			runLog.Warn("failed to persist assistant_final artifact", slog.String("run_id", current.GetRunId()), slog.String("error", saveErr.Error()))
		}
	}

	if err := s.finalizeWorkingMemory(ctx, event.SessionKey, current.GetRunId(), commonv1.RunState_RUN_STATE_COMPLETED, finalMessage); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("finalize working memory: %w", err))
	}
	if err := s.finalizeTransientWorkingState(ctx, event.SessionKey, current.GetRunId(), commonv1.RunState_RUN_STATE_COMPLETED, finalMessage); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("finalize transient working memory: %w", err))
	}

	if s.config.PipelineEnqueuer != nil {
		if enqueueErr := s.config.PipelineEnqueuer.EnqueuePostRun(ctx, current.GetRunId(), event.SessionKey); enqueueErr != nil {
			runLog.Warn("failed to enqueue memory pipeline job", slog.String("run_id", current.GetRunId()), slog.String("error", enqueueErr.Error()))
		} else {
			runLog.Info("memory pipeline job enqueued", slog.String("run_id", current.GetRunId()))
		}
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_COMPLETED, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run completed: %w", err))
	}

	runLog.Info("run completed", slog.String("session_key", event.SessionKey))
	s.emitEvent(next.GetRunId(), event.SessionKey, observability.EventRunCompleted, map[string]any{"response_length": len(finalMessage)})
	s.scheduleHubCleanup(next.GetRunId())

	return &ExecutionResult{RunID: next.GetRunId(), SessionKey: event.SessionKey, CurrentState: next.GetCurrentState(), AssistantResponse: finalMessage}, nil
}

func (s *Service) consumeModelStream(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, initialStream transport.EventStream) (string, *sessionv1.RunRecord, error) {
	var finalMessage string
	stream := initialStream

	for stream != nil {
		var nextStream transport.EventStream
		for transportEvent := range stream {
			var err error
			if current, err = s.persistProviderSessionRef(ctx, current, transportEvent.ProviderSessionRef, runLog); err != nil {
				return "", current, err
			}
			switch transportEvent.EventType {
			case transport.EventTypeAssistantDelta:
				if transportEvent.AssistantDelta != nil {
					if err := s.config.Delivery.DeliverAssistantDelta(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: transportEvent.AssistantDelta.Content, SequenceNo: transportEvent.AssistantDelta.SequenceNo}); err != nil {
						return "", current, err
					}
					s.emitEvent(current.GetRunId(), sessionKey, observability.EventAssistantDelta, map[string]any{"sequence_no": transportEvent.AssistantDelta.SequenceNo, "content_length": len(transportEvent.AssistantDelta.Content)})
				}
			case transport.EventTypeAssistantFinal:
				if transportEvent.AssistantFinal != nil {
					finalMessage = transportEvent.AssistantFinal.Content
					if err := s.config.Delivery.DeliverAssistantFinal(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: finalMessage, Final: true}); err != nil {
						return "", current, err
					}
					s.emitEvent(current.GetRunId(), sessionKey, observability.EventAssistantFinal, map[string]any{"content_length": len(finalMessage), "finish_reason": transportEvent.AssistantFinal.FinishReason})
				}
			case transport.EventTypeToolCallRequested:
				resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, transportEvent.ToolCall)
				if err != nil {
					return "", updated, err
				}
				current = updated
				nextStream = resumed
				break
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
				return "", current, transportError(transportEvent.TransportError)
			case transport.EventTypeTransportWarning:
				runLog.Warn("transport warning", slog.String("payload", transportEvent.PayloadJSON))
			}
		}
		stream = nextStream
	}
	return finalMessage, current, nil
}

func (s *Service) failRun(ctx context.Context, current *sessionv1.RunRecord, leaseID string, runLog *slog.Logger, err error) error {
	runLog.Error("run execution failed", slog.String("error", err.Error()))
	s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventRunError, map[string]any{
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
