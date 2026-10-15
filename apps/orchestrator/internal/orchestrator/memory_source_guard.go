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
		failed := func(err error, held *transport.TransportEvent) {
			// Preserve complete accounting metadata from the rejected event, never
			// its text/tool/final payload. Drain already-ready metadata before
			// cancellation so a ticker/proposal selection race cannot drop it.
			var usage *transport.Usage
			observe := func(event transport.TransportEvent) {
				if usage == nil {
					if u, e := modelTurnUsage(event); e == nil && u != nil {
						copy := *u
						usage = &copy
					}
				}
			}
			if held != nil {
				observe(*held)
			}
			closed := false
			remaining := 256
		ready:
			for remaining > 0 && usage == nil && !closed {
				select {
				case event, ok := <-stream:
					remaining--
					closed = !ok
					if ok {
						observe(event)
					}
				default:
					break ready
				}
			}
			cancel()
			// A cooperative provider may flush its complete turn usage while
			// cancelling. Cleanup is synchronous, bounded and has no detached
			// drain goroutine or permission to issue another provider request.
			if usage == nil && !closed && remaining > 0 {
				timer := time.NewTimer(100 * time.Millisecond)
				defer timer.Stop()
				for remaining > 0 && usage == nil && !closed {
					select {
					case event, ok := <-stream:
						remaining--
						closed = !ok
						if ok {
							observe(event)
						}
					case <-timer.C:
						remaining = 0
					}
				}
			}
			event := transport.NewTransportErrorEvent(runID, p.Name(), err)
			event.Usage = usage
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
					failed(e, nil)
					return
				}
			case event, ok := <-stream:
				if !ok {
					return
				}
				// Final/tool proposals must not pass using a cached source snapshot.
				if event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					if e := check(bounded); e != nil {
						failed(e, &event)
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
