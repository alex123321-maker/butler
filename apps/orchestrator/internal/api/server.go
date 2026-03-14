package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	orchestratorv1 "github.com/butler/butler/internal/gen/orchestrator/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Executor interface {
	Execute(context.Context, ingress.InputEvent) (*orchestrator.ExecutionResult, error)
}

type Server struct {
	orchestratorv1.UnimplementedOrchestratorServiceServer

	executor Executor
	log      *slog.Logger
}

func NewServer(executor Executor, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{executor: executor, log: logger.WithComponent(log, "orchestrator-api")}
}

func (s *Server) SubmitEvent(ctx context.Context, req *orchestratorv1.SubmitEventRequest) (*orchestratorv1.SubmitEventResponse, error) {
	if req.GetEvent() == nil {
		return nil, status.Error(codes.InvalidArgument, "event is required")
	}
	event, err := eventFromProto(req.GetEvent())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.executor.Execute(ctx, event)
	if err != nil {
		s.log.Error("submit event failed", slog.String("session_key", event.SessionKey), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &orchestratorv1.SubmitEventResponse{
		RunId:             result.RunID,
		SessionKey:        result.SessionKey,
		CurrentState:      result.CurrentState,
		AssistantResponse: result.AssistantResponse,
	}, nil
}

func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var request submitEventHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}
		event, err := request.toInputEvent()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		result, err := s.executor.Execute(r.Context(), event)
		if err != nil {
			s.log.Error("rest submit event failed", slog.String("session_key", event.SessionKey), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"run_id":             result.RunID,
			"session_key":        result.SessionKey,
			"current_state":      result.CurrentState.String(),
			"assistant_response": result.AssistantResponse,
		})
	})
}

type submitEventHTTPRequest struct {
	EventID        string          `json:"event_id"`
	EventType      any             `json:"event_type"`
	SessionKey     string          `json:"session_key"`
	Source         string          `json:"source"`
	PayloadJSON    string          `json:"payload_json"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      string          `json:"created_at"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (r submitEventHTTPRequest) toInputEvent() (ingress.InputEvent, error) {
	eventType, err := parseEventType(r.EventType)
	if err != nil {
		return ingress.InputEvent{}, err
	}
	payloadJSON := strings.TrimSpace(r.PayloadJSON)
	if payloadJSON == "" && len(r.Payload) > 0 {
		payloadJSON = string(r.Payload)
	}
	createdAt := time.Now().UTC()
	if strings.TrimSpace(r.CreatedAt) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
		if err != nil {
			return ingress.InputEvent{}, fmt.Errorf("created_at must be RFC3339")
		}
		createdAt = parsed.UTC()
	}
	return ingress.InputEvent{
		EventID:        strings.TrimSpace(r.EventID),
		EventType:      eventType,
		SessionKey:     strings.TrimSpace(r.SessionKey),
		Source:         strings.TrimSpace(r.Source),
		PayloadJSON:    payloadJSON,
		CreatedAt:      createdAt,
		IdempotencyKey: strings.TrimSpace(r.IdempotencyKey),
	}, nil
}

func eventFromProto(event *runv1.InputEvent) (ingress.InputEvent, error) {
	if event == nil {
		return ingress.InputEvent{}, fmt.Errorf("event is required")
	}
	createdAt := time.Now().UTC()
	if strings.TrimSpace(event.GetCreatedAt()) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, event.GetCreatedAt())
		if err != nil {
			return ingress.InputEvent{}, fmt.Errorf("event.created_at must be RFC3339")
		}
		createdAt = parsed.UTC()
	}
	return ingress.InputEvent{
		EventID:        strings.TrimSpace(event.GetEventId()),
		EventType:      event.GetEventType(),
		SessionKey:     strings.TrimSpace(event.GetSessionKey()),
		Source:         strings.TrimSpace(event.GetSource()),
		PayloadJSON:    strings.TrimSpace(event.GetPayloadJson()),
		CreatedAt:      createdAt,
		IdempotencyKey: strings.TrimSpace(event.GetIdempotencyKey()),
	}, nil
}

func parseEventType(value any) (runv1.InputEventType, error) {
	switch typed := value.(type) {
	case string:
		normalized := strings.TrimSpace(strings.ToUpper(typed))
		switch normalized {
		case "INPUT_EVENT_TYPE_USER_MESSAGE", "USER_MESSAGE":
			return runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, nil
		case "INPUT_EVENT_TYPE_UI_ACTION", "UI_ACTION":
			return runv1.InputEventType_INPUT_EVENT_TYPE_UI_ACTION, nil
		case "INPUT_EVENT_TYPE_SYSTEM_DIAGNOSTIC_TRIGGER", "SYSTEM_DIAGNOSTIC_TRIGGER":
			return runv1.InputEventType_INPUT_EVENT_TYPE_SYSTEM_DIAGNOSTIC_TRIGGER, nil
		case "INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT", "SCHEDULED_INTERNAL_EVENT":
			return runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, nil
		case "INPUT_EVENT_TYPE_RESUME_OR_RETRY_EVENT", "RESUME_OR_RETRY_EVENT":
			return runv1.InputEventType_INPUT_EVENT_TYPE_RESUME_OR_RETRY_EVENT, nil
		case "INPUT_EVENT_TYPE_APPROVAL_RESPONSE_EVENT", "APPROVAL_RESPONSE_EVENT":
			return runv1.InputEventType_INPUT_EVENT_TYPE_APPROVAL_RESPONSE_EVENT, nil
		default:
			return runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED, fmt.Errorf("event_type is invalid")
		}
	case float64:
		return runv1.InputEventType(int32(typed)), nil
	case nil:
		return runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED, fmt.Errorf("event_type is required")
	default:
		return runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED, fmt.Errorf("event_type is invalid")
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
