package orchestrator

import (
	"context"
	"errors"
	"testing"
)

type fakeObserver struct {
	finalCount    int
	approvalCount int
	lastFinalErr  error
}

func (f *fakeObserver) RecordAssistantDelta(ctx context.Context, event DeliveryEvent) {}
func (f *fakeObserver) RecordAssistantFinal(ctx context.Context, event DeliveryEvent, err error) {
	f.finalCount++
	f.lastFinalErr = err
}
func (f *fakeObserver) RecordApprovalRequest(ctx context.Context, req ApprovalRequest, err error) {
	f.approvalCount++
}
func (f *fakeObserver) RecordToolCallEvent(ctx context.Context, event ToolCallEvent, err error) {}
func (f *fakeObserver) RecordStatusEvent(ctx context.Context, event StatusEvent, err error)     {}

type fakeSink struct {
	err error
}

func (f fakeSink) DeliverAssistantDelta(context.Context, DeliveryEvent) error    { return nil }
func (f fakeSink) DeliverAssistantFinal(context.Context, DeliveryEvent) error    { return f.err }
func (f fakeSink) DeliverApprovalRequest(context.Context, ApprovalRequest) error { return nil }
func (f fakeSink) DeliverToolCallEvent(context.Context, ToolCallEvent) error     { return nil }
func (f fakeSink) DeliverStatusEvent(context.Context, StatusEvent) error         { return nil }

func TestObservedDeliverySinkRecordsEvenOnError(t *testing.T) {
	t.Parallel()

	observer := &fakeObserver{}
	errExpected := errors.New("delivery failed")
	sink := NewObservedDeliverySink(fakeSink{err: errExpected}, observer)

	err := sink.DeliverAssistantFinal(context.Background(), DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:1", Content: "done", Final: true})
	if err == nil {
		t.Fatal("expected delivery error")
	}
	if observer.finalCount != 1 {
		t.Fatalf("expected observer final count 1, got %d", observer.finalCount)
	}
	if observer.lastFinalErr == nil {
		t.Fatal("expected observer to receive delivery error")
	}
}
