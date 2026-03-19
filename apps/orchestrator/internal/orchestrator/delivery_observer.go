package orchestrator

import "context"

// DeliveryObserver receives delivery outcomes for durable channel visibility.
type DeliveryObserver interface {
	RecordAssistantDelta(ctx context.Context, event DeliveryEvent)
	RecordAssistantFinal(ctx context.Context, event DeliveryEvent, err error)
	RecordApprovalRequest(ctx context.Context, req ApprovalRequest, err error)
	RecordToolCallEvent(ctx context.Context, event ToolCallEvent, err error)
	RecordStatusEvent(ctx context.Context, event StatusEvent, err error)
}

type observedDeliverySink struct {
	base     DeliverySink
	observer DeliveryObserver
}

func NewObservedDeliverySink(base DeliverySink, observer DeliveryObserver) DeliverySink {
	if base == nil {
		base = NopDeliverySink{}
	}
	if observer == nil {
		return base
	}
	return observedDeliverySink{base: base, observer: observer}
}

func (s observedDeliverySink) DeliverAssistantDelta(ctx context.Context, event DeliveryEvent) error {
	err := s.base.DeliverAssistantDelta(ctx, event)
	s.observer.RecordAssistantDelta(ctx, event)
	return err
}

func (s observedDeliverySink) DeliverAssistantFinal(ctx context.Context, event DeliveryEvent) error {
	err := s.base.DeliverAssistantFinal(ctx, event)
	s.observer.RecordAssistantFinal(ctx, event, err)
	return err
}

func (s observedDeliverySink) DeliverApprovalRequest(ctx context.Context, req ApprovalRequest) error {
	err := s.base.DeliverApprovalRequest(ctx, req)
	s.observer.RecordApprovalRequest(ctx, req, err)
	return err
}

func (s observedDeliverySink) DeliverToolCallEvent(ctx context.Context, event ToolCallEvent) error {
	err := s.base.DeliverToolCallEvent(ctx, event)
	s.observer.RecordToolCallEvent(ctx, event, err)
	return err
}

func (s observedDeliverySink) DeliverStatusEvent(ctx context.Context, event StatusEvent) error {
	err := s.base.DeliverStatusEvent(ctx, event)
	s.observer.RecordStatusEvent(ctx, event, err)
	return err
}
