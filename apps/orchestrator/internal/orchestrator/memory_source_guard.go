package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/butler/butler/internal/transport"
)

type sourceGuardedProvider struct {
	transport.ModelProvider
	sources interface {
		CheckRunSources(context.Context, string) error
	}
}

func (p *sourceGuardedProvider) InferenceLimits() (int64, int64) {
	if bounded, ok := p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }); ok {
		return bounded.InferenceLimits()
	}
	return 0, 0
}

func (p *sourceGuardedProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.Context.RunID, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(c, r) })
}
func (p *sourceGuardedProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.RunID, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(c, r) })
}
func (p *sourceGuardedProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.RunID, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.SubmitToolResult(c, r) })
}

func (p *sourceGuardedProvider) guard(ctx context.Context, runID string, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	// Compound owning lifecycle checks retain all current source/authority
	// barriers. Their explicit trusted bound cannot exceed the private skill
	// verifier's existing two-second bound, and never changes event admission.
	checkTimeout := 200 * time.Millisecond
	if bounded, ok := p.sources.(interface{ MemorySourceCheckTimeout() time.Duration }); ok {
		requested := bounded.MemorySourceCheckTimeout()
		if requested < checkTimeout || requested > 2*time.Second {
			return nil, errors.New("invalid owning memory source verification bound")
		}
		checkTimeout = requested
	}
	check := func(ctx context.Context) error {
		bounded, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		if err := p.sources.CheckRunSources(bounded, runID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			kind := transport.ErrorTypeMemorySourceStale
			if errors.Is(err, context.DeadlineExceeded) || bounded.Err() == context.DeadlineExceeded {
				kind = transport.ErrorTypeMemorySourceTimeout
			}
			return &transport.Error{Type: kind, Message: string(kind)}
		}
		return nil
	}
	if e := check(ctx); e != nil {
		return nil, e
	}
	bounded, cancel := context.WithCancel(ctx)
	stream, e := call(bounded)
	if e != nil {
		cancel()
		return nil, e
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		failed := func(err error) {
			event := transport.NewTransportErrorEvent(runID, p.Name(), err)
			select {
			case out <- event:
			case <-ctx.Done():
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e := check(bounded); e != nil {
					failed(e)
					return
				}
			case event, ok := <-stream:
				if !ok {
					return
				}
				// Final/tool proposals must not pass using a cached source snapshot.
				if event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					if e := check(bounded); e != nil {
						failed(e)
						return
					}
				}
				select {
				case out <- event:
				case <-ctx.Done():
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
