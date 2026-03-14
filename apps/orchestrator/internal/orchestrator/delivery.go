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
