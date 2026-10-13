package app

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

// All actual root/child entries share one window. The kernel budget wrapper
// remains the owning reservation ledger; this is an additional acceptance cap.
type releaseChainV4Provider struct {
	transport.ModelProvider
	checker         *releaseChainEntryChecker
	window          *releaseChainV4Window
	scenario        string
	expectedVersion string
	before          func(context.Context, string, string) error
}

func (*releaseChainV4Provider) InferenceLimits() (int64, int64) { return 1048576, 32768 }

func (p *releaseChainV4Provider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || r.Context.SessionKey != token.SessionKey || r.Context.ProviderName != antigravity.Name || r.Context.ModelName != antigravity.FlashHighModel {
		return nil, errNativeFixtureDenied
	}
	return p.entry(ctx, "start", r.Context.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(call, r) })
}
func (p *releaseChainV4Provider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.entry(ctx, "continue", r.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(call, r) })
}
func (p *releaseChainV4Provider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.entry(ctx, "submit", r.RunID, r, func(call context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(call, r)
	})
}

func (p *releaseChainV4Provider) entry(ctx context.Context, kind, run string, input any, invoke func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	if p == nil || p.checker == nil || p.ModelProvider == nil || p.window == nil || p.expectedVersion == "" {
		return nil, errNativeFixtureDenied
	}
	if p.before != nil {
		if err := p.before(ctx, kind, run); err != nil {
			return nil, err
		}
	}
	ordinal, deadline, err := p.window.reserve(ctx, p.scenario, kind, p.checker.root, run, input, p.checker.guardTx)
	if err != nil {
		return nil, err
	}
	// This bound owns the whole auth/preflight/invoke/stream entry. The native
	// HTTP timeout is only an additional transport limit, not its replacement.
	attemptDeadline := time.Now().Add(120 * time.Second)
	if attemptDeadline.Before(deadline) {
		deadline = attemptDeadline
	}
	call, cancel := context.WithDeadline(ctx, deadline)
	if call.Err() != nil {
		p.finish(ordinal, "cancelled", nil, nil, "entry", transport.NormalizeError(call.Err(), p.Name()))
		cancel()
		return nil, call.Err()
	}
	stream, err := invoke(call)
	if err != nil {
		p.finish(ordinal, "transport_error", nil, nil, "entry", transport.NormalizeError(err, p.Name()))
		cancel()
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		outcome := "transport_error"
		failure := &transport.Error{Type: transport.ErrorTypeProviderProtocolError}
		var observed *transport.Usage
		var version *string
		var pendingFinal *transport.TransportEvent
		observedComplete := false
		usageUncertain, versionUncertain := false, false
		defer func() {
			if !observedComplete || usageUncertain {
				observed = nil
			}
			if !observedComplete || versionUncertain {
				version = nil
			}
			p.finish(ordinal, outcome, observed, version, "stream", failure)
		}()
		for {
			select {
			case <-call.Done():
				outcome = "cancelled"
				failure = transport.NormalizeError(call.Err(), p.Name())
				return
			case event, ok := <-stream:
				if !ok {
					return
				}
				if event.RunID != run {
					event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("provider returned a foreign run"))
				}
				usage := event.Usage
				if event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
					if usage != nil && *usage != *event.AssistantFinal.Usage {
						usageUncertain = true
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("ambiguous native usage"))
						usage = nil
					} else {
						usage = event.AssistantFinal.Usage
					}
				}
				if usage != nil {
					completeUsage := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeRunCompleted || event.EventType == transport.EventTypeTransportError
					if !completeUsage || usage.Validate() != nil || observed != nil && *observed != *usage {
						usageUncertain = true
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("invalid native usage observation"))
					} else if observed == nil {
						copy := *usage
						observed = &copy
					}
				}
				if v := event.ProviderModelVersion; v != nil {
					completeVersion := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeRunCompleted || event.EventType == transport.EventTypeTransportError
					if !completeVersion || *v != p.expectedVersion || version != nil && *version != *v {
						versionUncertain = true
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("native model version differs from frozen prerequisite"))
					} else {
						copy := *v
						version = &copy
					}
				}
				// The provider parsed this complete response packet before emitting
				// it. Later EOF/cancellation is a lifecycle failure, not evidence
				// that uniquely validated packet metadata was never observed.
				if event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallBatchRequested {
					observedComplete = true
				}
				// A tool or final answer can lead to another request or an effect.
				// Stop before releasing it if this prospective experiment cannot
				// retain the complete cost and selected version. A failed request
				// still retains any unambiguous dimensions it actually reported.
				actionable := event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeAssistantFinal
				fullTotal := observed != nil && (observed.TotalTokens > 0 || observed.InputTokens == 0 && observed.OutputTokens == 0)
				if actionable && (usageUncertain || versionUncertain || !fullTotal || version == nil) {
					event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("native complete metadata unavailable"))
				}
				if event.EventType == transport.EventTypeToolCallBatchRequested {
					event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("native tool batches are unsupported"))
				}
				complete := event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeRunCompleted
				if complete || event.EventType == transport.EventTypeTransportError {
					observedComplete = true
				}
				if complete {
					outcome = "completed"
					failure = nil
				} else if event.EventType == transport.EventTypeTransportError {
					failure = event.TransportError
				}
				if event.EventType == transport.EventTypeAssistantFinal {
					copy := event
					pendingFinal = &copy
					continue
				}
				if event.EventType == transport.EventTypeRunCompleted && pendingFinal != nil {
					select {
					case out <- *pendingFinal:
					case <-call.Done():
						outcome = "cancelled"
						failure = transport.NormalizeError(call.Err(), p.Name())
						return
					}
				}
				select {
				case out <- event:
				case <-call.Done():
					outcome = "cancelled"
					failure = transport.NormalizeError(call.Err(), p.Name())
					return
				}
				if complete || event.EventType == transport.EventTypeTransportError {
					return
				}
			}
		}
	}()
	return out, nil
}

// A measurement never creates authority or refunds an attempt. Unknown or
// incomplete cost remains NULL. The one terminal row cannot be overwritten.
func (p *releaseChainV4Provider) finish(ordinal int, outcome string, usage *transport.Usage, version *string, stage string, failure *transport.Error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var in, out, total *int64
	if usage != nil {
		a, b := int64(usage.InputTokens), int64(usage.OutputTokens)
		in, out = &a, &b
		if usage.TotalTokens != 0 || usage.InputTokens == 0 && usage.OutputTokens == 0 {
			c := int64(usage.TotalTokens)
			total = &c
		}
	}
	var failureStage, code *string
	if outcome != "completed" {
		if stage != "entry" && stage != "stream" {
			stage = "stream"
		}
		c := transport.DiagnosticCode(failure)
		if c == "" {
			c = string(transport.ErrorTypeInternalTransportError)
		}
		failureStage, code = &stage, &c
	}
	_, _ = p.window.pool.Exec(ctx, `UPDATE `+p.window.table("fixture_chain_v4_high_requests")+` SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,model_version=$6,finished_at=clock_timestamp(),elapsed_ms=GREATEST(0,floor(extract(epoch FROM clock_timestamp()-admitted_at)*1000)::bigint),failure_stage=$7,diagnostic_code=$8 WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, outcome, in, out, total, version, failureStage, code)
}
