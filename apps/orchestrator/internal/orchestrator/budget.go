package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/budget"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/transport"
)

type budgetedProvider struct {
	transport.ModelProvider
	service *Service
}

func (p *budgetedProvider) Capabilities(ctx context.Context, c transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	cap, err := p.ModelProvider.Capabilities(ctx, c)
	cap.SupportsBatchToolCalls = false
	return cap, err
}
func (p *budgetedProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	return p.admit(ctx, r.Context.RunID, func(turn context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(turn, r) })
}
func (p *budgetedProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.admit(ctx, r.RunID, func(turn context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(turn, r) })
}
func (p *budgetedProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.admit(ctx, r.RunID, func(turn context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(turn, r)
	})
}
func (p *budgetedProvider) admit(ctx context.Context, runID string, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	s := p.service
	limits, ok := p.ModelProvider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return nil, &budget.LoopError{Reason: "provider_inference_limits_unavailable"}
	}
	input, output := limits.InferenceLimits()
	if input <= 0 || output <= 0 {
		return nil, &budget.LoopError{Reason: "provider_inference_limits_invalid"}
	}
	if output > s.config.Budgets.Policy.OutputTokens {
		return nil, &budget.LoopError{Reason: "provider_output_allowance_exceeds_policy"}
	}
	a, err := s.config.Budgets.AdmitModel(ctx, runID, s.config.ActionMandateID, s.config.ProviderName, s.config.ModelName, input)
	if err != nil {
		return nil, err
	}
	modelTurnLimited := modelTurnRetryBound(ctx, a)
	origin := "scope_or_authority"
	if modelTurnLimited {
		origin = "model_turn"
	}
	if retryStore, ok := s.config.Checkpoints.(modelRetryStore); ok && ctx.Value(modelRetryRecoveryKey{}) == true {
		if err = retryStore.MarkModelRetryStarted(ctx, runID, a.OperationID); err != nil {
			return nil, err
		}
	}
	if err = s.checkpoint(ctx, runID, "model_budget_reserved", map[string]any{"operation_id": a.OperationID, "reservation_id": a.ReservationID, "deadline": a.Deadline, "deadline_origin": origin}); err != nil {
		return nil, err
	}
	turn, cancel := context.WithDeadline(ctx, a.Deadline)
	if turn.Err() != nil {
		cancel()
		return nil, &budget.LoopError{Reason: "owner_authority_deadline"}
	}
	turn = transport.WithInputByteLimit(turn, s.config.Budgets.Policy.ModelInputBytes)
	turn = budget.WithModelOperation(turn, a.OperationID)
	turn, phases := withModelRetryStages(turn)
	stream, err := call(turn)
	if err != nil {
		if modelTurnLimited && phases.inferencePhase() && ctx.Err() == nil && (errors.Is(err, context.DeadlineExceeded) || transport.DiagnosticCode(err) == string(transport.ErrorTypeProviderTimeout)) {
			err = &modelTurnTimeout{operationID: a.OperationID}
		} else if errors.Is(turn.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			// The owning budget still expires if inference was never observed.
			// Early provider timeouts and caller cancellation do not acquire
			// this budget deadline classification.
			err = &budget.LoopError{Reason: "execution_budget_deadline"}
		}
		cancel()
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		deadlineEvent := func() {
			if ctx.Err() == nil {
				event := transport.NewTransportErrorEvent(runID, p.Name(), fmt.Errorf("execution budget deadline"))
				event.TransportError.Type = "execution_budget_deadline"
				if modelTurnLimited && phases.inferencePhase() {
					event.TransportError.Type = "model_turn_timeout"
				}
				select {
				case out <- event:
				case <-ctx.Done():
				}
			}
		}
		var observed *transport.Usage
		for {
			select {
			case <-turn.Done():
				deadlineEvent()
				return
			case event, ok := <-stream:
				if !ok {
					if turn.Err() != nil {
						deadlineEvent()
					}
					return
				}
				// Only this trusted admission wrapper emits retry eligibility.
				// Provider codes/messages cannot identify an owning operation.
				if event.EventType == transport.EventTypeRunTimedOut || event.EventType == transport.EventTypeTransportError && event.TransportError != nil && (event.TransportError.Type == transport.ErrorTypeProviderTimeout || event.TransportError.Type == "model_turn_timeout") {
					event = modelRetryTimeoutEvent(event, modelTurnLimited && event.RunID == runID && phases.inferencePhase(), ctx.Err() == nil)
				}
				usage, usageErr := modelTurnUsage(event)
				stopTurn := false
				if usageErr == nil && observed != nil && usage != nil && *usage != *observed {
					usageErr = fmt.Errorf("conflicting observations for one model turn")
				}
				if usageErr != nil {
					stopTurn = true
					event = transport.NewTransportErrorEvent(runID, p.Name(), fmt.Errorf("invalid observed model usage"))
				} else if usage != nil && observed == nil {
					copy := *usage
					observed = &copy
					// Preserve the provider's complete numeric observation for operator
					// comparisons. Candidates alone omit High's thinking consumption.
					// No prompt, response, reasoning or provider credential is logged.
					s.log.Info("model turn usage observed", "run_id", runID, "operation_id", a.OperationID,
						"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "total_tokens", usage.TotalTokens)
					in, output := int64(usage.InputTokens), int64(usage.OutputTokens)
					observeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
					err := s.config.Budgets.ObserveModel(observeCtx, a.OperationID, &in, &output)
					stop()
					if err != nil {
						s.log.Warn("model usage persistence unavailable; conservative reservation retained")
					}
					generated := output
					// High's native total includes prompt, candidates and thinking;
					// its generation ceiling bounds candidates+thinking together.
					// Other adapters may include cached dimensions in their total.
					if s.config.ProviderName == modelprovider.ProviderAntigravity && s.config.ModelName == "gemini-3.8-flash-high" && usage.TotalTokens > 0 {
						generated = max(generated, int64(usage.TotalTokens)-in)
					}
					if in > input || generated > a.OutputTokens {
						stopTurn = true
						event = transport.NewTransportErrorEvent(runID, p.Name(), fmt.Errorf("provider exceeded declared inference allowance"))
						event.TransportError.Type = "execution_budget_tokens"
					}
				}
				select {
				case out <- event:
				case <-turn.Done():
					deadlineEvent()
					return
				}
				if event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					return
				}
				if stopTurn {
					return
				}
			}
		}
	}()
	return out, nil
}

// Prefer one turn-level observation, with the existing final-only field as a
// compatibility path. Neither field is cumulative usage across multiple turns.
func modelTurnUsage(event transport.TransportEvent) (*transport.Usage, error) {
	usage := event.Usage
	if final := event.AssistantFinal; final != nil && final.Usage != nil {
		if usage != nil && *usage != *final.Usage {
			return nil, fmt.Errorf("ambiguous observed model usage")
		}
		if usage == nil {
			usage = final.Usage
		}
	}
	if usage == nil {
		return nil, nil
	}
	switch event.EventType {
	case transport.EventTypeAssistantFinal, transport.EventTypeToolCallRequested, transport.EventTypeToolCallBatchRequested, transport.EventTypeTransportError, transport.EventTypeRunCompleted, transport.EventTypeRunCancelled, transport.EventTypeRunTimedOut:
	default:
		return nil, fmt.Errorf("usage is not a complete turn observation")
	}
	if err := usage.Validate(); err != nil {
		return nil, err
	}
	return usage, nil
}

func budgetStopReason(err error) (string, bool) {
	var exhausted *budget.ExhaustedError
	if errors.As(err, &exhausted) {
		return "budget_" + exhausted.Dimension + "_" + exhausted.Kind, true
	}
	var loop *budget.LoopError
	if errors.As(err, &loop) {
		return loop.Reason, true
	}
	if errors.Is(err, budget.ErrUnknownCost) {
		return "model_cost_basis_unavailable", true
	}
	if errors.Is(err, budget.ErrConflict) {
		return "budget_policy_or_identity_conflict", true
	}
	var transportError *transport.Error
	if errors.As(err, &transportError) && transportError.Type == transport.ErrorTypeInputBytesExceeded {
		return string(transport.ErrorTypeInputBytesExceeded), true
	}
	return "", false
}

// Stop preserves the preceding checkpoint as a continuation and does not invoke
// the completion coordinator or turn an exhausted allowance into success.
func (s *Service) stopBudget(ctx context.Context, current *sessionv1.RunRecord, err error) (*ExecutionResult, error) {
	reason, ok := budgetStopReason(err)
	if !ok {
		return nil, err
	}
	stage, payload := "", ""
	if s.config.Checkpoints != nil {
		var e error
		stage, payload, e = s.config.Checkpoints.LoadCheckpoint(ctx, current.GetRunId())
		if e != nil {
			return nil, e
		}
	}
	if err = s.checkpoint(ctx, current.GetRunId(), "execution_stopped", map[string]any{"reason": reason, "continuation_stage": stage, "continuation_json": payload}); err != nil {
		return nil, err
	}
	_, _ = s.provider.CancelRun(ctx, transport.CancelRunRequest{RunID: current.GetRunId(), Reason: reason})
	s.blockTaskRun(ctx, current.GetRunId(), reason)
	return &ExecutionResult{RunID: current.GetRunId(), SessionKey: current.GetSessionKey(), CurrentState: current.GetCurrentState(), ReceiptStatus: "blocked", ErrorCode: reason}, nil
}
