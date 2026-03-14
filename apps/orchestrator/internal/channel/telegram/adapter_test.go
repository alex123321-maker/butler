package telegram

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeUpdateBuildsUserMessageEvent(t *testing.T) {
	t.Parallel()

	event, err := NormalizeUpdate(Update{
		UpdateID: 42,
		Message: &Message{
			MessageID: 100,
			Date:      1710500000,
			Text:      " hello telegram ",
			Chat:      Chat{ID: 777},
			From:      &User{ID: 999, Username: "alice", FirstName: "Alice"},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeUpdate returned error: %v", err)
	}
	if event.SessionKey != "telegram:chat:777" {
		t.Fatalf("unexpected session key %q", event.SessionKey)
	}
	if event.Source != "telegram" {
		t.Fatalf("unexpected source %q", event.Source)
	}
	if event.IdempotencyKey != "telegram-update-42" {
		t.Fatalf("unexpected idempotency key %q", event.IdempotencyKey)
	}
	if event.CreatedAt.UTC() != time.Unix(1710500000, 0).UTC() {
		t.Fatalf("unexpected created_at %s", event.CreatedAt)
	}
}

func TestNormalizeUpdateIgnoresUnsupportedUpdates(t *testing.T) {
	t.Parallel()

	_, err := NormalizeUpdate(Update{UpdateID: 1})
	if !errors.Is(err, ErrIgnoredUpdate) {
		t.Fatalf("expected ErrIgnoredUpdate for missing message, got %v", err)
	}

	_, err = NormalizeUpdate(Update{UpdateID: 2, Message: &Message{Chat: Chat{ID: 1}}})
	if !errors.Is(err, ErrIgnoredUpdate) {
		t.Fatalf("expected ErrIgnoredUpdate for empty text, got %v", err)
	}
}

func TestChatIDRoundTrip(t *testing.T) {
	t.Parallel()

	chatID, ok := ChatIDFromSessionKey(SessionKeyFromChatID(123456))
	if !ok {
		t.Fatal("expected session key to parse")
	}
	if chatID != 123456 {
		t.Fatalf("unexpected chat id %d", chatID)
	}
	if _, ok := ChatIDFromSessionKey("not-telegram"); ok {
		t.Fatal("expected non-telegram session key to be rejected")
	}
}
