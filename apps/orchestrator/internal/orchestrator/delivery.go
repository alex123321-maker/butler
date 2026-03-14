package orchestrator

import (
	"context"
	"log/slog"

	"github.com/butler/butler/internal/logger"
)

type DeliverySink interface {
	DeliverAssistantDelta(context.Context, DeliveryEvent) error
	DeliverAssistantFinal(context.Context, DeliveryEvent) error
}

type NopDeliverySink struct{}

func (NopDeliverySink) DeliverAssistantDelta(context.Context, DeliveryEvent) error { return nil }

func (NopDeliverySink) DeliverAssistantFinal(context.Context, DeliveryEvent) error { return nil }

type LoggingDeliverySink struct {
	log *slog.Logger
}

type CompositeDeliverySink struct {
	sinks []DeliverySink
}

func NewLoggingDeliverySink(log *slog.Logger) LoggingDeliverySink {
	if log == nil {
		log = slog.Default()
	}
	return LoggingDeliverySink{log: logger.WithComponent(log, "delivery")}
}

func (s LoggingDeliverySink) DeliverAssistantDelta(_ context.Context, event DeliveryEvent) error {
	s.log.Info("assistant delta delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey), slog.Int("sequence_no", event.SequenceNo))
	return nil
}

func (s LoggingDeliverySink) DeliverAssistantFinal(_ context.Context, event DeliveryEvent) error {
	s.log.Info("assistant final delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey))
	return nil
}

func NewCompositeDeliverySink(sinks ...DeliverySink) CompositeDeliverySink {
	filtered := make([]DeliverySink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return CompositeDeliverySink{sinks: filtered}
}

func (s CompositeDeliverySink) DeliverAssistantDelta(ctx context.Context, event DeliveryEvent) error {
	for _, sink := range s.sinks {
		if err := sink.DeliverAssistantDelta(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s CompositeDeliverySink) DeliverAssistantFinal(ctx context.Context, event DeliveryEvent) error {
	for _, sink := range s.sinks {
		if err := sink.DeliverAssistantFinal(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
