package deliveryevents

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RecordAssistantDelta(ctx context.Context, event flow.DeliveryEvent) {
	s.record(ctx, CreateParams{RunID: event.RunID, SessionKey: event.SessionKey, Channel: channelFromSessionKey(event.SessionKey), DeliveryType: TypeAssistantDelta, State: StateSent, DetailsJSON: mustJSON(map[string]any{"sequence_no": event.SequenceNo, "chars": len(event.Content)}), CreatedAt: time.Now().UTC()})
}

func (s *Service) RecordAssistantFinal(ctx context.Context, event flow.DeliveryEvent, err error) {
	state := StateSent
	errMsg := ""
	if err != nil {
		state = StateFailed
		errMsg = err.Error()
	}
	s.record(ctx, CreateParams{RunID: event.RunID, SessionKey: event.SessionKey, Channel: channelFromSessionKey(event.SessionKey), DeliveryType: TypeAssistantFinal, State: state, ErrorMessage: errMsg, DetailsJSON: mustJSON(map[string]any{"final": event.Final, "chars": len(event.Content)}), CreatedAt: time.Now().UTC()})
}

func (s *Service) RecordApprovalRequest(ctx context.Context, req flow.ApprovalRequest, err error) {
	state := StateWaiting
	errMsg := ""
	if err != nil {
		state = StateFailed
		errMsg = err.Error()
	}
	s.record(ctx, CreateParams{RunID: req.RunID, SessionKey: req.SessionKey, Channel: channelFromSessionKey(req.SessionKey), DeliveryType: TypeApprovalRequest, State: state, ErrorMessage: errMsg, DetailsJSON: mustJSON(map[string]any{"tool_call_id": req.ToolCallID, "tool_name": req.ToolName}), CreatedAt: time.Now().UTC()})
}

func (s *Service) RecordToolCallEvent(ctx context.Context, event flow.ToolCallEvent, err error) {
	state := StateSent
	errMsg := ""
	if err != nil {
		state = StateFailed
		errMsg = err.Error()
	}
	s.record(ctx, CreateParams{RunID: event.RunID, SessionKey: event.SessionKey, Channel: channelFromSessionKey(event.SessionKey), DeliveryType: TypeToolCallEvent, State: state, ErrorMessage: errMsg, DetailsJSON: mustJSON(map[string]any{"tool_call_id": event.ToolCallID, "tool_name": event.ToolName, "status": event.Status, "duration_ms": event.DurationMs}), CreatedAt: time.Now().UTC()})
}

func (s *Service) RecordStatusEvent(ctx context.Context, event flow.StatusEvent, err error) {
	state := StateSent
	errMsg := ""
	if err != nil {
		state = StateFailed
		errMsg = err.Error()
	}
	s.record(ctx, CreateParams{RunID: event.RunID, SessionKey: event.SessionKey, Channel: channelFromSessionKey(event.SessionKey), DeliveryType: TypeStatus, State: state, ErrorMessage: errMsg, DetailsJSON: mustJSON(map[string]any{"status": event.Status}), CreatedAt: time.Now().UTC()})
}

func (s *Service) record(ctx context.Context, params CreateParams) {
	if s == nil || s.repo == nil {
		return
	}
	_, _ = s.repo.CreateEvent(ctx, params)
}

func channelFromSessionKey(sessionKey string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sessionKey)), "telegram:") {
		return ChannelTelegram
	}
	return ChannelWeb
}

func mustJSON(value map[string]any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
