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
type chainHighProvider struct {
	transport.ModelProvider
	checker  *releaseChainEntryChecker
	window   *chainHighWindow
	scenario string
	before   func(context.Context, string, string) error
}

func (*chainHighProvider) InferenceLimits() (int64, int64) { return 1048576, 32768 }

func (p *chainHighProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || r.Context.SessionKey != token.SessionKey || r.Context.ProviderName != antigravity.Name || r.Context.ModelName != antigravity.FlashHighModel {
		return nil, errNativeFixtureDenied
	}
	return p.entry(ctx, "start", r.Context.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(call, r) })
}
func (p *chainHighProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.entry(ctx, "continue", r.RunID, r, func(call context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(call, r) })
}
func (p *chainHighProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.entry(ctx, "submit", r.RunID, r, func(call context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(call, r)
	})
}

func (p *chainHighProvider) entry(ctx context.Context, kind, run string, input any, invoke func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	if p == nil || p.checker == nil || p.ModelProvider == nil || p.window == nil {
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
	call, cancel := context.WithDeadline(ctx, deadline)
	if call.Err() != nil {
		p.finish(ordinal, "cancelled", nil, nil)
		cancel()
		return nil, call.Err()
	}
	stream, err := invoke(call)
	if err != nil {
		p.finish(ordinal, "transport_error", nil, nil)
		cancel()
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		outcome := "transport_error"
		var observed *transport.Usage
		var version *string
		observedComplete := false
		defer func() {
			if !observedComplete {
				observed, version = nil, nil
			}
			p.finish(ordinal, outcome, observed, version)
		}()
		for {
			select {
			case <-call.Done():
				outcome = "cancelled"
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
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("ambiguous native usage"))
						usage = nil
					} else {
						usage = event.AssistantFinal.Usage
					}
				}
				if usage != nil {
					completeUsage := event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeRunCompleted || event.EventType == transport.EventTypeTransportError
					if !completeUsage || usage.Validate() != nil || observed != nil && *observed != *usage {
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("invalid native usage observation"))
					} else if observed == nil {
						copy := *usage
						observed = &copy
					}
				}
				if v := event.ProviderModelVersion; v != nil {
					if *v != "gemini-3.8-flash-n" || version != nil && *version != *v {
						event = transport.NewTransportErrorEvent(run, p.Name(), errors.New("native model version differs from frozen prerequisite"))
					} else {
						copy := *v
						version = &copy
					}
				}
				complete := event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeRunCompleted
				if complete {
					observedComplete = true
					outcome = "completed"
				}
				select {
				case out <- event:
				case <-call.Done():
					outcome = "cancelled"
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
func (p *chainHighProvider) finish(ordinal int, outcome string, usage *transport.Usage, version *string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var in, out, total *int64
	if usage != nil {
		a, b := int64(usage.InputTokens), int64(usage.OutputTokens)
		in, out = &a, &b
		if usage.TotalTokens != 0 {
			c := int64(usage.TotalTokens)
			total = &c
		}
	}
	_, _ = p.window.pool.Exec(ctx, `UPDATE `+p.window.table("fixture_chain_high_requests")+` SET outcome=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5,model_version=$6 WHERE ordinal=$1 AND outcome='unavailable'`, ordinal, outcome, in, out, total, version)
}
