package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

type diagnosticSources struct {
	err        error
	calls      atomic.Int32
	afterFirst bool
}

func (s *diagnosticSources) CheckRunSources(context.Context, string) error {
	if s.calls.Add(1) == 1 && s.afterFirst {
		return nil
	}
	return s.err
}

func TestSourceGuardDiagnosticsAtEveryProviderEntry(t *testing.T) {
	for _, entry := range []string{"start", "continue", "tool_result"} {
		for _, timedOut := range []bool{false, true} {
			t.Run(entry+"/"+map[bool]string{false: "stale", true: "timeout"}[timedOut], func(t *testing.T) {
				err := errors.New("PRIVATE_SOURCE_DIAGNOSTIC_MARKER")
				want := transport.ErrorTypeMemorySourceStale
				if timedOut {
					err = context.DeadlineExceeded
					want = transport.ErrorTypeMemorySourceTimeout
				}
				provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
				guard := &sourceGuardedProvider{provider, &diagnosticSources{err: err}}
				var got error
				switch entry {
				case "start":
					_, got = guard.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
				case "continue":
					_, got = guard.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "run"})
				case "tool_result":
					_, got = guard.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run"})
				}
				if transport.DiagnosticCode(got) != string(want) || provider.ctx != nil || strings.Contains(got.Error(), "PRIVATE_") {
					t.Fatal("source check did not deny with a safe diagnostic")
				}
			})
		}
	}
}

func TestSourceGuardDiagnosesLateTimeoutWithoutPublishingFinal(t *testing.T) {
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	sources := &diagnosticSources{err: context.DeadlineExceeded, afterFirst: true}
	guard := &sourceGuardedProvider{provider, sources}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := guard.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	provider.stream <- transport.TransportEvent{EventType: transport.EventTypeAssistantFinal}
	event := <-stream
	if event.EventType != transport.EventTypeTransportError || event.TransportError.Type != transport.ErrorTypeMemorySourceTimeout {
		t.Fatal("late timeout released final or lost its diagnostic")
	}
	if _, ok := <-stream; ok {
		t.Fatal("failed source stream remained open")
	}
	<-provider.ctx.Done()
}
