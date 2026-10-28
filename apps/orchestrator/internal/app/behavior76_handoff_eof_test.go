package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func TestBehavior76LocalHTTPHandoffRequiresEOFFirst(t *testing.T) {
	for _, cancelBeforeEOF := range []bool{false, true} {
		t.Run(map[bool]string{false: "consumer_cancels_after_handoff", true: "parent_cancels_before_EOF"}[cancelBeforeEOF], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var global atomic.Bool
			p := &behavior76NativeProvider{sealed: &global}
			stream := make(chan transport.TransportEvent)
			proposalSent := make(chan struct{})
			eof := make(chan struct{})
			go func() {
				defer close(stream)
				stream <- transport.NewEvent("r", transport.EventTypeRunStarted, "public")
				stream <- transport.NewToolCallRequestedEvent("r", "public", transport.ToolCallRequest{ToolCallRef: "c", ToolName: "http.request", ArgsJSON: `{}`})
				close(proposalSent)
				<-eof
			}()
			finished := false
			proposalSeen := false
			out := p.observeStream(ctx, stream, func(status string, _ []map[string]any, _ *transport.Usage, _ string) {
				finished = true
				if !cancelBeforeEOF && status != "tool_handoff" {
					t.Error("ready handoff lost terminal status")
				}
			})
			<-proposalSent
			if cancelBeforeEOF {
				cancel()
			}
			close(eof)
			for event := range out {
				if event.EventType == transport.EventTypeToolCallRequested {
					proposalSeen = true
					if !finished {
						t.Error("proposal preceded exact EOF/journal finish")
					}
					cancel()
				}
			}
			if !finished || (!cancelBeforeEOF && (!proposalSeen || global.Load())) || (cancelBeforeEOF && (proposalSeen || !global.Load())) {
				t.Fatal("handoff/cancellation boundary widened", proposalSeen, global.Load(), finished)
			}
		})
	}
}

func TestBehavior76LocalHTTPHandoffLateFailureIsGlobal(t *testing.T) {
	var global atomic.Bool
	p := &behavior76NativeProvider{sealed: &global}
	proposal := transport.NewToolCallRequestedEvent("r", "public", transport.ToolCallRequest{ToolCallRef: "c", ToolName: "http.request", ArgsJSON: `{}`})
	e := transport.NewEvent("r", transport.EventTypeTransportError, "public")
	e.TransportError = &transport.Error{Type: transport.ErrorTypeProviderUnavailable}
	stream := initiativeProjectStream(transport.NewEvent("r", transport.EventTypeRunStarted, "public"), proposal, e)
	proposalSeen := false
	recorded := false
	for event := range p.observeStream(t.Context(), stream, func(status string, events []map[string]any, _ *transport.Usage, _ string) {
		recorded = len(events) == 3 && status == "transport_error"
	}) {
		if event.EventType == transport.EventTypeToolCallRequested {
			proposalSeen = true
		}
	}
	if !global.Load() || proposalSeen || !recorded {
		t.Fatal("late fault released proposal or discarded evidence", global.Load(), proposalSeen, recorded)
	}
}

func TestBehavior76LocalHTTPHandoffReservesCompleteTail(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var global atomic.Bool
	p := &behavior76NativeProvider{sealed: &global}
	// Both prefix entries remain queued until actual EOF and journal finish.
	// Publishing the proposal must reserve room for its optional terminal too.
	prefix := transport.NewEvent("r", transport.EventTypeAssistantDelta, "public")
	proposal := transport.NewToolCallRequestedEvent("r", "public", transport.ToolCallRequest{ToolCallRef: "c", ToolName: "http.request", ArgsJSON: `{}`})
	stream := initiativeProjectStream(transport.NewEvent("r", transport.EventTypeRunStarted, "public"), prefix, proposal, transport.NewEvent("r", transport.EventTypeRunCompleted, "public"))
	finished := make(chan struct{})
	out := p.observeStream(ctx, stream, func(status string, events []map[string]any, _ *transport.Usage, _ string) {
		if status != "tool_handoff" || len(events) != 4 {
			t.Error("complete raw tail was not journalled before publication")
		}
		close(finished)
	})
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("provider EOF/journal blocked behind queued prefix")
	}
	var types []transport.EventType
	for event := range out {
		types = append(types, event.EventType)
		if event.EventType == transport.EventTypeToolCallRequested {
			cancel() // Owning wrapper cancels immediately upon receiving proposal.
		}
	}
	if global.Load() || len(types) != 4 || types[2] != transport.EventTypeToolCallRequested || types[3] != transport.EventTypeRunCompleted {
		t.Fatal("queued prefix/optional terminal widened handoff cancellation", global.Load(), types)
	}
}
