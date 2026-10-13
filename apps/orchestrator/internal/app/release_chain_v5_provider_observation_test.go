package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestReleaseChainV5ProviderObservationFacts(t *testing.T) {
	final := func(reason string) transport.TransportEvent {
		return transport.NewAssistantFinalEvent("synthetic-run", antigravity.Name, transport.AssistantFinal{FinishReason: reason})
	}
	terminal := func(kind transport.EventType) transport.TransportEvent {
		return transport.NewTerminalEvent("synthetic-run", kind, antigravity.Name)
	}
	for _, tc := range []struct {
		name                               string
		events                             []transport.TransportEvent
		finals, stop, max                  int
		completed, cancel, timeout, errors int
		reason                             string
		uncertain                          bool
	}{
		{"completed_final", []transport.TransportEvent{final("STOP"), terminal(transport.EventTypeRunCompleted)}, 1, 1, 0, 1, 0, 0, 0, "STOP", false},
		{"truncated_final", []transport.TransportEvent{final("MAX_TOKENS"), terminal(transport.EventTypeRunCompleted)}, 1, 0, 1, 1, 0, 0, 0, "MAX_TOKENS", false},
		{"conflicting_finish_reasons", []transport.TransportEvent{final("STOP"), final("MAX_TOKENS")}, 2, 1, 1, 0, 0, 0, 0, "", true},
		{"private_reason_not_retained", []transport.TransportEvent{final("PRIVATE_PROVIDER_TEXT")}, 1, 0, 0, 0, 0, 0, 0, "", true},
		{"all_original_terminal_kinds", []transport.TransportEvent{terminal(transport.EventTypeRunCompleted), terminal(transport.EventTypeRunCancelled), terminal(transport.EventTypeRunTimedOut), terminal(transport.EventTypeTransportError)}, 0, 0, 0, 1, 1, 1, 1, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &releaseChainV5Observations{}
			for _, event := range tc.events {
				o.observe(event)
			}
			if o.finals != tc.finals || o.stopFinals != tc.stop || o.maxTokensFinals != tc.max || o.completed != tc.completed || o.cancelled != tc.cancel || o.timedOut != tc.timeout || o.transportErrors != tc.errors || o.terminals != tc.completed+tc.cancel+tc.timeout+tc.errors || o.finishUncertain != tc.uncertain {
				t.Fatal("original lifecycle/finish counters differ")
			}
			if (o.finishReason != nil) != (tc.reason != "") || o.finishReason != nil && *o.finishReason != tc.reason {
				t.Fatal("uncertain or private finish text became an exact reason")
			}
		})
	}
}

func TestReleaseChainV5FailureOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"own_deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "timed_out"},
		{"own_cancel", fmt.Errorf("wrapped: %w", context.Canceled), "cancelled"},
		{"provider_error", &transport.Error{Type: transport.ErrorTypeProviderUnavailable}, "transport_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if releaseChainV5FailureOutcome(tc.err) != tc.want {
				t.Fatal("own context state lost or provider error invented context cancellation")
			}
		})
	}
}
