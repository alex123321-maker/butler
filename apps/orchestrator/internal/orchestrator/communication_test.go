package orchestrator

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
	"testing"
	"time"
)

type communicationKernelFixture struct {
	decisions []string
	calls     int
}

func (*communicationKernelFixture) Explicit(context.Context, string) (bool, error) { return true, nil }
func (k *communicationKernelFixture) BeforeFinal(context.Context, string, string) (string, error) {
	n := k.calls
	k.calls++
	if n < len(k.decisions) {
		return k.decisions[n], nil
	}
	return "ready", nil
}
func (*communicationKernelFixture) PendingInteraction(context.Context, string) (string, error) {
	return "", nil
}
func (*communicationKernelFixture) ResumeInteraction(context.Context, string, string) (string, error) {
	return "", errors.New("none")
}
func TestCommunicationRequiresFinalAndTerminalAndSuppressesDeltas(t *testing.T) {
	for _, scenario := range []string{"empty_terminal", "missing_terminal", "missing_final"} {
		t.Run(scenario, func(t *testing.T) {
			sink := &unavailablePartial{}
			kernel := &communicationKernelFixture{}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, &mockProvider{}, Config{Communication: kernel, Delivery: sink}, nil)
			ch := make(chan transport.TransportEvent, 3)
			ch <- transport.NewAssistantDeltaEvent("run", "fixture", transport.AssistantDelta{Content: "private delta"})
			if scenario != "missing_final" {
				ch <- transport.NewAssistantFinalEvent("run", "fixture", transport.AssistantFinal{Content: "", FinishReason: "stop"})
			}
			if scenario != "missing_terminal" {
				ch <- transport.NewTerminalEvent("run", transport.EventTypeRunCompleted, "fixture")
			}
			close(ch)
			_, _, e := s.consumeModelStream(withAnswerState(context.Background(), ""), s.log, &sessionv1.RunRecord{RunId: "run", SessionKey: "web:test", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}, "web:test", "lease", ch)
			if (e == nil) != (scenario == "empty_terminal") {
				t.Fatal("typed terminal semantics", scenario, e)
			}
			if sink.calls != 0 || sink.finals != 0 {
				t.Fatal("implicit model output delivered")
			}
		})
	}
}

type communicationProvider struct {
	mockProvider
	continuations                    int
	failCorrection, abruptCorrection bool
}

func (p *communicationProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	p.continuations++
	if p.failCorrection {
		return nil, errors.New("fixture correction unavailable")
	}
	ch := make(chan transport.TransportEvent, 2)
	if p.abruptCorrection {
		close(ch)
		return ch, nil
	}
	ch <- transport.NewAssistantFinalEvent("", "fixture", transport.AssistantFinal{Content: "", FinishReason: "stop"})
	ch <- transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "fixture")
	close(ch)
	return ch, nil
}

func TestCommunicationCorrectionFailureClosesObligation(t *testing.T) {
	for _, phase := range []string{"admission", "stream"} {
		t.Run(phase, func(t *testing.T) {
			kernel := &communicationKernelFixture{decisions: []string{"correct", "failed"}}
			writer := &communicationCompletion{}
			provider := &communicationProvider{failCorrection: phase == "admission", abruptCorrection: phase == "stream", mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "fixture", transport.AssistantFinal{Content: "not published", FinishReason: "stop"}), transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "fixture")}}}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{Communication: kernel, Completion: writer}, nil)
			r, e := s.Execute(context.Background(), ingress.InputEvent{EventID: "event", IdempotencyKey: "event", SessionKey: "web:test", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"answer"}`, CreatedAt: time.Now()})
			if e != nil || r.ReceiptStatus != "blocked" || r.ErrorCode != "communication_result_missing" || kernel.calls != 2 || provider.continuations != 1 || len(writer.requests) != 0 {
				t.Fatal("failed single correction abandoned an open obligation", r, e, kernel.calls, provider.continuations)
			}
		})
	}
}

type communicationCompletion struct{ requests []completion.Request }

func (c *communicationCompletion) Save(_ context.Context, q completion.Request) (completion.FinalRecord, error) {
	c.requests = append(c.requests, q)
	return completion.FinalRecord{RunID: q.RunID, SessionKey: q.SessionKey, Content: q.Content, ResultID: "result:" + q.RunID, AnswerState: q.AnswerState, Verdict: "unverified"}, nil
}
func TestCommunicationSingleCorrectionAndInternalEmptyFinal(t *testing.T) {
	kernel := &communicationKernelFixture{decisions: []string{"correct", "ready"}}
	writer := &communicationCompletion{}
	sink := &unavailablePartial{}
	provider := &communicationProvider{mockProvider: mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "fixture", transport.AssistantFinal{Content: "not published", FinishReason: "stop"}), transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "fixture")}}}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{Communication: kernel, Completion: writer, Delivery: sink}, nil)
	r, e := s.Execute(context.Background(), ingress.InputEvent{EventID: "event", IdempotencyKey: "event", SessionKey: "web:test", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"answer"}`, CreatedAt: time.Now()})
	if e != nil || r.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || provider.continuations != 1 || len(writer.requests) != 1 || writer.requests[0].Content != "" || !writer.requests[0].TerminalObserved || sink.calls != 0 || sink.finals != 0 {
		t.Fatal("correction/final boundary", r, e, provider.continuations, writer.requests)
	}
}
