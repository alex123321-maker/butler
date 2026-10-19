package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

func TestBrowserOwnerFencePrecedesInferenceProposalAndProviderEvent(t *testing.T) {
	blocked := errors.New("owner control active")
	checks := 0
	s := &Service{config: Config{WaitBrowserOwner: func(_ context.Context, run string) error {
		if run != "run" {
			t.Fatal("wrong run")
		}
		checks++
		return blocked
	}}}
	provider := &budgetedProvider{service: s} // nil transport must never be reached
	ctx := context.Background()
	if _, err := provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}}); !errors.Is(err, blocked) {
		t.Fatal(err)
	}
	if _, err := provider.ContinueRun(ctx, transport.ContinueRunRequest{RunID: "run"}); !errors.Is(err, blocked) {
		t.Fatal(err)
	}
	if _, err := provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "run"}); !errors.Is(err, blocked) {
		t.Fatal(err)
	}
	current := &sessionv1.RunRecord{RunId: "run"}
	if _, _, err := s.handleActionToolCall(ctx, slog.Default(), current, "lease", &transport.ToolCallRequest{ToolName: "browser.click"}, ""); !errors.Is(err, blocked) {
		t.Fatal(err)
	}
	events := make(chan transport.TransportEvent, 1)
	events <- transport.TransportEvent{EventType: transport.EventTypeAssistantFinal}
	close(events)
	if result, _, err := s.consumeModelStream(ctx, slog.Default(), current, "session", "lease", events); !errors.Is(err, blocked) || result != "" {
		t.Fatal("provider event escaped owner fence", result, err)
	}
	if checks != 5 {
		t.Fatal("owner fence missing at execution boundary", checks)
	}
}
