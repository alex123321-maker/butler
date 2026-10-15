package orchestrator

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

type blockedSourceProbe struct {
	checks  atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (p *blockedSourceProbe) CheckRunSources(ctx context.Context, _ string) error {
	if p.checks.Add(1) == 1 {
		return nil
	}
	close(p.entered)
	select {
	case <-p.release:
		return context.DeadlineExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestSourceGuardTickerRaceKeepsUsageAndReleasesNoData(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 2)}
	probe := &blockedSourceProbe{entered: make(chan struct{}), release: make(chan struct{})}
	guard := &sourceGuardedProvider{ModelProvider: provider, sources: probe}
	out, err := guard.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-probe.entered:
	case <-ctx.Done():
		t.Fatal("periodic check did not start")
	}
	usage := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 7}
	proposal := transport.NewToolCallRequestedEvent("run", "fixture", transport.ToolCallRequest{ToolName: "PRIVATE_PROPOSAL", ArgsJSON: `{"text":"PRIVATE_SOURCE_BODY"}`})
	proposal.Usage = &usage
	provider.stream <- proposal
	close(probe.release)
	event := <-out
	if event.EventType != transport.EventTypeTransportError || event.TransportError.Type != transport.ErrorTypeMemorySourceTimeout || event.Usage == nil || *event.Usage != usage || event.ToolCall != nil || event.AssistantFinal != nil || strings.Contains(event.PayloadJSON, "PRIVATE_") {
		t.Fatal("ticker/proposal race lost usage or released denied DATA", event.EventType)
	}
	if _, ok := <-out; ok {
		t.Fatal("denied guard released another event")
	}
	select {
	case <-provider.ctx.Done():
	case <-ctx.Done():
		t.Fatal("denied provider was not cancelled")
	}
}

func TestSourceGuardMissingUsageDrainIsBoundedAndJoins(t *testing.T) {
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	provider.stream <- transport.NewToolCallRequestedEvent("run", "fixture", transport.ToolCallRequest{ToolName: "PRIVATE_TOOL"})
	guard := &sourceGuardedProvider{ModelProvider: provider, sources: &diagnosticSources{afterFirst: true, err: context.DeadlineExceeded}}
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	started := time.Now()
	out, err := guard.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	event := <-out
	if event.EventType != transport.EventTypeTransportError || event.Usage != nil || event.ToolCall != nil || time.Since(started) > 500*time.Millisecond {
		t.Fatal("missing usage became known or cleanup escaped its bound")
	}
	if _, ok := <-out; ok {
		t.Fatal("drain did not join before guard close")
	}
	select {
	case <-provider.ctx.Done():
	default:
		t.Fatal("provider context still live after joined guard")
	}
}

func TestSourceGuardDoesNotAttributeAnotherRunUsage(t *testing.T) {
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	event := transport.NewToolCallRequestedEvent("other-run", "fixture", transport.ToolCallRequest{ToolName: "PRIVATE_TOOL"})
	event.Usage = &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	provider.stream <- event
	close(provider.stream)
	guard := &sourceGuardedProvider{ModelProvider: provider, sources: &diagnosticSources{afterFirst: true, err: context.DeadlineExceeded}}
	out, err := guard.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	denied := <-out
	if denied.EventType != transport.EventTypeTransportError || denied.Usage != nil || denied.ToolCall != nil {
		t.Fatal("other-run metadata became this admission's usage")
	}
	if _, ok := <-out; ok {
		t.Fatal("other-run rejected event did not close")
	}
}
