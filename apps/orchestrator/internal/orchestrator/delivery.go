package orchestrator

import (
	"context"
	"log/slog"

	"github.com/butler/butler/internal/logger"
)

// ApprovalRequest represents a tool call that needs user approval before execution.
type ApprovalRequest struct {
	RunID         string
	SessionKey    string
	ApprovalID    string
	ApprovalType  string
	ToolCallID    string
	ToolName      string
	ArgsJSON      string
	PayloadJSON   string
	TabCandidates []ApprovalTabCandidate
}

type ApprovalTabCandidate struct {
	CandidateToken string
	Title          string
	Domain         string
	CurrentURL     string
	FaviconURL     string
	DisplayLabel   string
	Status         string
}

// ApprovalResponse represents the user's decision on an approval request.
type ApprovalResponse struct {
	ToolCallID string
	Approved   bool
	Channel    string
	ResolvedBy string
}

// ToolCallEvent represents a tool call lifecycle event delivered to the channel.
type ToolCallEvent struct {
	RunID      string
	SessionKey string
	ToolCallID string
	ToolName   string
	ArgsJSON   string // set on started
	Status     string // "started" | "completed" | "failed"
	DurationMs int64  // set on completed
}

// StatusEvent represents a short status notification delivered to the channel
// so the user gets immediate feedback about what the bot is doing.
type StatusEvent struct {
	RunID      string
	SessionKey string
	Status     string // e.g. "thinking", "preparing"
}

type DeliverySink interface {
	DeliverAssistantDelta(context.Context, DeliveryEvent) error
	DeliverAssistantFinal(context.Context, DeliveryEvent) error
	DeliverApprovalRequest(context.Context, ApprovalRequest) error
	DeliverToolCallEvent(context.Context, ToolCallEvent) error
	DeliverStatusEvent(context.Context, StatusEvent) error
}

type NopDeliverySink struct{}

func (NopDeliverySink) DeliverAssistantDelta(context.Context, DeliveryEvent) error { return nil }

func (NopDeliverySink) DeliverAssistantFinal(context.Context, DeliveryEvent) error { return nil }

func (NopDeliverySink) DeliverApprovalRequest(context.Context, ApprovalRequest) error { return nil }

func (NopDeliverySink) DeliverToolCallEvent(context.Context, ToolCallEvent) error { return nil }

func (NopDeliverySink) DeliverStatusEvent(context.Context, StatusEvent) error { return nil }

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

func (s LoggingDeliverySink) DeliverApprovalRequest(_ context.Context, req ApprovalRequest) error {
	s.log.Info("approval request delivered",
		slog.String("run_id", req.RunID),
		slog.String("session_key", req.SessionKey),
		slog.String("approval_id", req.ApprovalID),
		slog.String("approval_type", req.ApprovalType),
		slog.String("tool_name", req.ToolName),
		slog.String("tool_call_id", req.ToolCallID),
	)
	return nil
}

func (s LoggingDeliverySink) DeliverToolCallEvent(_ context.Context, event ToolCallEvent) error {
	s.log.Info("tool call event delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey), slog.String("tool_name", event.ToolName), slog.String("status", event.Status))
	return nil
}

func (s LoggingDeliverySink) DeliverStatusEvent(_ context.Context, event StatusEvent) error {
	s.log.Info("status event delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey), slog.String("status", event.Status))
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

func (s CompositeDeliverySink) DeliverApprovalRequest(ctx context.Context, req ApprovalRequest) error {
	for _, sink := range s.sinks {
		if err := sink.DeliverApprovalRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (s CompositeDeliverySink) DeliverToolCallEvent(ctx context.Context, event ToolCallEvent) error {
	for _, sink := range s.sinks {
		if err := sink.DeliverToolCallEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s CompositeDeliverySink) DeliverStatusEvent(ctx context.Context, event StatusEvent) error {
	for _, sink := range s.sinks {
		if err := sink.DeliverStatusEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
