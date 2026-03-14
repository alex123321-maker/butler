package ingress

import (
	"testing"
	"time"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
)

func TestNormalizeEventBuildsNormalizedShape(t *testing.T) {
	createdAt := time.Date(2026, time.March, 15, 14, 0, 0, 0, time.UTC)
	event, err := NormalizeEvent(NormalizeRequest{
		EventType:  runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey: "telegram:chat:42",
		Source:     "telegram",
		Payload: map[string]any{
			"text": "hello",
			"user": "u-1",
		},
		CreatedAt:  createdAt,
		DeliveryID: "telegram-update-1",
	})
	if err != nil {
		t.Fatalf("NormalizeEvent returned error: %v", err)
	}
	if event.EventID == "" || event.IdempotencyKey == "" {
		t.Fatal("expected event identifiers to be assigned")
	}
	if event.CreatedAt != createdAt {
		t.Fatalf("expected created_at to be preserved, got %s", event.CreatedAt)
	}
	if event.PayloadJSON != `{"text":"hello","user":"u-1"}` {
		t.Fatalf("unexpected payload json %q", event.PayloadJSON)
	}
}

func TestNormalizeEventIdempotencyKeyIsDeterministic(t *testing.T) {
	first, err := NormalizeEvent(NormalizeRequest{
		EventType:  runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey: "telegram:chat:42",
		Source:     "telegram",
		Payload:    map[string]any{"text": "hello", "user": "u-1"},
		DeliveryID: "delivery-1",
	})
	if err != nil {
		t.Fatalf("NormalizeEvent returned error: %v", err)
	}
	second, err := NormalizeEvent(NormalizeRequest{
		EventType:  runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey: "telegram:chat:42",
		Source:     "telegram",
		Payload:    map[string]any{"user": "u-1", "text": "hello"},
		DeliveryID: "delivery-1",
	})
	if err != nil {
		t.Fatalf("NormalizeEvent returned error: %v", err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("expected deterministic idempotency key, got %q vs %q", first.IdempotencyKey, second.IdempotencyKey)
	}
}

func TestNormalizeEventRejectsMissingFields(t *testing.T) {
	_, err := NormalizeEvent(NormalizeRequest{Source: "telegram"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
