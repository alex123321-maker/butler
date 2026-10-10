package orchestrator

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/transport"
	"time"
)

const skillInvalidReason = "skill_revoked_or_incompatible"

// The live catalog check precedes budget admission and provider release. It
// uses a bounded private RPC outside ownership/source PostgreSQL transactions.
type skillProtectedProvider struct {
	transport.ModelProvider
	catalog SkillCatalog
}

func (p *skillProtectedProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.Context.RunID, func(turn context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(turn, r) })
}
func (p *skillProtectedProvider) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.RunID, func(turn context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(turn, r) })
}
func (p *skillProtectedProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.guard(ctx, r.RunID, func(turn context.Context) (transport.EventStream, error) {
		return p.ModelProvider.SubmitToolResult(turn, r)
	})
}
func (p *skillProtectedProvider) guard(ctx context.Context, run string, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	check := func() (bool, error) {
		verify, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		pins, err := p.catalog.Pinned(verify, run)
		return len(pins) > 0, err
	}
	loaded, err := check()
	if err != nil {
		return nil, &budget.LoopError{Reason: skillInvalidReason}
	}
	turn, cancel := context.WithCancel(ctx)
	stream, err := call(turn)
	if err != nil {
		cancel()
		return nil, err
	}
	if !loaded { // A load tool result enters the next fresh checked model turn.
		out := make(chan transport.TransportEvent)
		go func() {
			defer close(out)
			defer cancel()
			for {
				var event transport.TransportEvent
				select {
				case <-turn.Done():
					return
				case value, ok := <-stream:
					if !ok {
						return
					}
					event = value
				}
				select {
				case out <- event:
				case <-turn.Done():
					return
				}
				if event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					return
				}
			}
		}()
		return out, nil
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		defer cancel()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		deny := func() {
			cancel()
			event := transport.NewTransportErrorEvent(run, p.Name(), fmt.Errorf("loaded procedural data is no longer valid"))
			event.TransportError.Type = skillInvalidReason
			select {
			case out <- event:
			case <-ctx.Done():
			}
		}
		for {
			select {
			case <-turn.Done():
				return
			case <-ticker.C:
				if _, err := check(); err != nil {
					deny()
					return
				}
			case event, ok := <-stream:
				if !ok {
					return
				}
				if _, err := check(); err != nil {
					deny()
					return
				}
				select {
				case out <- event:
				case <-turn.Done():
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
