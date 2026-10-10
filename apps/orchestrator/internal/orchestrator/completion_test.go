package orchestrator

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
	"testing"
	"time"
)

type unavailablePartial struct {
	NopDeliverySink
	calls  int
	finals int
}

func (s *unavailablePartial) DeliverAssistantDelta(context.Context, DeliveryEvent) error {
	s.calls++
	return errors.New("channel unavailable")
}
func (s *unavailablePartial) DeliverAssistantFinal(context.Context, DeliveryEvent) error {
	s.finals++
	return errors.New("channel unavailable")
}

type completionRecorder struct {
	inner    completion.Writer
	requests []completion.Request
}

func (c *completionRecorder) Save(ctx context.Context, q completion.Request) (completion.FinalRecord, error) {
	c.requests = append(c.requests, q)
	return c.inner.Save(ctx, q)
}
func TestPartialChannelFailureDoesNotInterruptAndTruncationRemainsExplicit(t *testing.T) {
	sink := &unavailablePartial{}
	runs := newMemoryRunManager()
	provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantDeltaEvent("", "fixture", transport.AssistantDelta{Content: "one"}), transport.NewAssistantDeltaEvent("", "fixture", transport.AssistantDelta{Content: "two"}), transport.NewAssistantFinalEvent("", "fixture", transport.AssistantFinal{Content: "partial answer", FinishReason: "MAX_TOKENS"})}}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, provider, Config{Delivery: sink}, nil)
	recorder := &completionRecorder{inner: s.config.Completion}
	s.config.Completion = recorder
	result, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "event", IdempotencyKey: "event", SessionKey: "web:test", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"do work"}`, CreatedAt: time.Now()})
	if err != nil || result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED {
		t.Fatal("channel failure stopped persistence", err)
	}
	if sink.calls != 1 || len(recorder.requests) != 1 || recorder.requests[0].AnswerState != "partial" || result.Verdict != "unverified" {
		t.Fatalf("wrong delivery/partial semantics: %#v %#v", result, recorder.requests)
	}
}
func TestAtomicCompletionDependencyFailsClosed(t *testing.T) {
	runs := newMemoryRunManager()
	s := NewService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "fixture", transport.AssistantFinal{Content: "done"})}}, Config{}, nil)
	_, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "event", IdempotencyKey: "event", SessionKey: "web:test", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"do work"}`, CreatedAt: time.Now()})
	if err == nil || runs.statesFor("run-1")[len(runs.statesFor("run-1"))-1] != commonv1.RunState_RUN_STATE_FINALIZING {
		t.Fatal("missing coordinator reported completed")
	}
}
