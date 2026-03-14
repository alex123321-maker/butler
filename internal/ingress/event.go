package ingress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
)

type InputEvent struct {
	EventID        string
	EventType      runv1.InputEventType
	SessionKey     string
	Source         string
	PayloadJSON    string
	CreatedAt      time.Time
	IdempotencyKey string
}

type NormalizeRequest struct {
	EventID    string
	EventType  runv1.InputEventType
	SessionKey string
	Source     string
	Payload    any
	CreatedAt  time.Time
	DeliveryID string
}

func NormalizeEvent(req NormalizeRequest) (InputEvent, error) {
	if req.EventType == runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED {
		return InputEvent{}, fmt.Errorf("event_type is required")
	}
	if strings.TrimSpace(req.SessionKey) == "" {
		return InputEvent{}, fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(req.Source) == "" {
		return InputEvent{}, fmt.Errorf("source is required")
	}
	payloadJSON, err := normalizePayload(req.Payload)
	if err != nil {
		return InputEvent{}, err
	}
	createdAt := req.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	idempotencyKey := buildIdempotencyKey(req.EventType, req.SessionKey, req.Source, payloadJSON, req.DeliveryID)
	eventID := strings.TrimSpace(req.EventID)
	if eventID == "" {
		eventID = idempotencyKey
	}

	return InputEvent{
		EventID:        eventID,
		EventType:      req.EventType,
		SessionKey:     strings.TrimSpace(req.SessionKey),
		Source:         strings.TrimSpace(req.Source),
		PayloadJSON:    payloadJSON,
		CreatedAt:      createdAt,
		IdempotencyKey: idempotencyKey,
	}, nil
}

func (e InputEvent) ToProto() *runv1.InputEvent {
	return &runv1.InputEvent{
		EventId:        e.EventID,
		EventType:      e.EventType,
		SessionKey:     e.SessionKey,
		Source:         e.Source,
		PayloadJson:    e.PayloadJSON,
		CreatedAt:      e.CreatedAt.UTC().Format(time.RFC3339Nano),
		IdempotencyKey: e.IdempotencyKey,
	}
}

func normalizePayload(payload any) (string, error) {
	if payload == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("normalize payload: %w", err)
	}
	return string(encoded), nil
}

func buildIdempotencyKey(eventType runv1.InputEventType, sessionKey, source, payloadJSON, deliveryID string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(eventType.String()))
	_, _ = hasher.Write([]byte("\n" + strings.TrimSpace(sessionKey)))
	_, _ = hasher.Write([]byte("\n" + strings.TrimSpace(source)))
	_, _ = hasher.Write([]byte("\n" + payloadJSON))
	_, _ = hasher.Write([]byte("\n" + strings.TrimSpace(deliveryID)))
	return hex.EncodeToString(hasher.Sum(nil))
}
