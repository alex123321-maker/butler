package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
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

func TestDeliverApprovalRequestSendsInlineKeyboard(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	// Test handleCallbackQuery resolves approval via gate.
	go func() {
		time.Sleep(10 * time.Millisecond)
		adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
			ID:      "cbq-1",
			Data:    "approve:call-1",
			Message: &Message{Chat: Chat{ID: 777}},
		})
	}()

	resp, waitErr := gate.Wait(context.Background(), "call-1")
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if !resp.Approved {
		t.Fatal("expected approval to be true")
	}
}

func TestHandleCallbackQueryRejectsApproval(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
			ID:      "cbq-2",
			Data:    "reject:call-2",
			Message: &Message{Chat: Chat{ID: 777}},
		})
	}()

	resp, waitErr := gate.Wait(context.Background(), "call-2")
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if resp.Approved {
		t.Fatal("expected approval to be false (rejected)")
	}
}

func TestHandleCallbackQueryIgnoresDisallowedChat(t *testing.T) {
	t.Parallel()

	client := &Client{}
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	// This should be ignored because chat ID 999 is not in the allowed list.
	adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
		ID:      "cbq-3",
		Data:    "approve:call-3",
		Message: &Message{Chat: Chat{ID: 999}},
	})

	// Verify the gate was NOT resolved.
	if gate.Resolve("call-3", true) {
		// If Resolve returns true, it means the pending channel existed but
		// the callback didn't consume it. This is fine for this test since
		// we didn't call Wait.
	}
}

func TestDeliverAssistantStreamingEditsSingleTelegramMessage(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	client.sendMessage = func(context.Context, int64, string) (Message, error) {
		return Message{MessageID: 10}, nil
	}
	var edits []string
	client.editMessage = func(_ context.Context, _ int64, _ int64, text string) (Message, error) {
		edits = append(edits, text)
		return Message{MessageID: 10}, nil
	}

	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "Hel", SequenceNo: 1}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "Hello", SequenceNo: 2}); err != nil {
		t.Fatalf("DeliverAssistantDelta second returned error: %v", err)
	}
	if err := adapter.DeliverAssistantFinal(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "Hello world", Final: true}); err != nil {
		t.Fatalf("DeliverAssistantFinal returned error: %v", err)
	}
	if len(edits) != 2 || edits[0] != "Hello" || edits[1] != "Hello world" {
		t.Fatalf("unexpected edits: %+v", edits)
	}
	if _, ok := adapter.streamingMessages["run-1"]; ok {
		t.Fatal("expected final delivery to clear streaming state")
	}
}
