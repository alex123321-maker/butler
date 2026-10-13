package app

import (
	"context"
	"errors"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

type taskAttemptDeliveryReader interface {
	AttemptDeliveryAllowed(context.Context, string, string) (bool, error)
}

// Wrap the outward sink before its channel-delivery observer: suppression must
// not mint a sent delivery event. Core execution/activity history remains
// observable. Approval requests and explicit interventions are never hidden.
type verifiedTaskDelivery struct {
	sink  flow.DeliverySink
	tasks taskAttemptDeliveryReader
}

func newVerifiedTaskDelivery(sink flow.DeliverySink, tasks taskAttemptDeliveryReader) flow.DeliverySink {
	return verifiedTaskDelivery{sink: sink, tasks: tasks}
}
func (d verifiedTaskDelivery) allowed(ctx context.Context, runID, key string) (bool, error) {
	if d.sink == nil || d.tasks == nil {
		return false, errors.New("current task delivery unavailable")
	}
	return d.tasks.AttemptDeliveryAllowed(ctx, runID, key)
}
func (d verifiedTaskDelivery) DeliverAssistantDelta(ctx context.Context, e flow.DeliveryEvent) error {
	allowed, err := d.allowed(ctx, e.RunID, e.SessionKey)
	if err != nil || !allowed {
		return err
	}
	return d.sink.DeliverAssistantDelta(ctx, e)
}
func (d verifiedTaskDelivery) DeliverAssistantFinal(ctx context.Context, e flow.DeliveryEvent) error {
	allowed, err := d.allowed(ctx, e.RunID, e.SessionKey)
	if err != nil || !allowed {
		return err
	}
	return d.sink.DeliverAssistantFinal(ctx, e)
}
func (d verifiedTaskDelivery) DeliverApprovalRequest(ctx context.Context, e flow.ApprovalRequest) error {
	if d.sink == nil {
		return errors.New("channel approval delivery unavailable")
	}
	return d.sink.DeliverApprovalRequest(ctx, e)
}
func (d verifiedTaskDelivery) DeliverToolCallEvent(ctx context.Context, e flow.ToolCallEvent) error {
	allowed, err := d.allowed(ctx, e.RunID, e.SessionKey)
	if err != nil {
		return err
	}
	if allowed {
		return d.sink.DeliverToolCallEvent(ctx, e)
	}
	return nil
}
func (d verifiedTaskDelivery) DeliverStatusEvent(ctx context.Context, e flow.StatusEvent) error {
	allowed, err := d.allowed(ctx, e.RunID, e.SessionKey)
	if err != nil {
		return err
	}
	if allowed || e.Status == "blocked" || e.Status == "waiting_for_user" {
		return d.sink.DeliverStatusEvent(ctx, e)
	}
	return nil
}
