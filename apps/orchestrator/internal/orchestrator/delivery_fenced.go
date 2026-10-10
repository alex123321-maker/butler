package orchestrator

import (
	"context"

	"github.com/butler/butler/internal/ownership"
)

type fencedDeliverySink struct{ base DeliverySink }

func (s fencedDeliverySink) DeliverAssistantDelta(ctx context.Context, event DeliveryEvent) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.base.DeliverAssistantDelta(ctx, event)
}
func (s fencedDeliverySink) DeliverAssistantFinal(ctx context.Context, event DeliveryEvent) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.base.DeliverAssistantFinal(ctx, event)
}
func (s fencedDeliverySink) DeliverApprovalRequest(ctx context.Context, event ApprovalRequest) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.base.DeliverApprovalRequest(ctx, event)
}
func (s fencedDeliverySink) DeliverToolCallEvent(ctx context.Context, event ToolCallEvent) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.base.DeliverToolCallEvent(ctx, event)
}
func (s fencedDeliverySink) DeliverStatusEvent(ctx context.Context, event StatusEvent) error {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.base.DeliverStatusEvent(ctx, event)
}
