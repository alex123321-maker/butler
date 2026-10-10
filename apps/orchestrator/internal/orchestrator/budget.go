package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/budget"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
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
	if err = s.checkpoint(ctx, runID, "model_budget_reserved", map[string]any{"operation_id": a.OperationID, "reservation_id": a.ReservationID, "deadline": a.Deadline}); err != nil {
		return nil, err
	}
	turn, cancel := context.WithDeadline(ctx, a.Deadline)
	turn = transport.WithInputByteLimit(turn, s.config.Budgets.Policy.ModelInputBytes)
	stream, err := call(turn)
	if err != nil {
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
				select {
				case out <- event:
				case <-ctx.Done():
				}
			}
		}
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
				if final := event.AssistantFinal; final != nil && final.Usage != nil {
					in, output := int64(final.Usage.InputTokens), int64(final.Usage.OutputTokens)
					observeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
					err := s.config.Budgets.ObserveModel(observeCtx, a.OperationID, &in, &output)
					stop()
					if err != nil {
						s.log.Warn("model usage persistence unavailable; conservative reservation retained")
					}
					if in > input || output > a.OutputTokens {
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
			}
		}
	}()
	return out, nil
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
