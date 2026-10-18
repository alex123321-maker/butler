package mobile

import (
	"fmt"
	"testing"
	"time"
)

func TestMessagePartsAndNarrowNoise(t *testing.T) {
	text := "Запись в четверг в 15:00"
	sender := "Барбер"
	channel := "appointments"
	e := Event{Change: "POSTED", Package: "example.test", Channel: &channel, Posted: 123, Payload: &Payload{Text: &text, Title: &sender}}
	if len(MessageParts(e)) != 1 {
		t.Fatal("plain message missing")
	}
	e.Payload.Progress = &Progress{Current: 30, Max: 100}
	if len(MessageParts(e)) != 0 {
		t.Fatal("progress-only update counted as a message")
	}
	e.Payload.Progress = nil
	e.Payload.Messages = []Message{{Sender: &sender, Text: text, Timestamp: 123}, {Text: "Перенос на пятницу", Timestamp: 124}}
	if len(MessageParts(e)) != 2 {
		t.Fatal("MessagingStyle items must count individually")
	}
	e.Change = "REMOVED"
	if len(MessageParts(e)) != 0 {
		t.Fatal("dismissal is not a new message")
	}
	e.Change = "UPDATED"
	rule := NoiseRule{Package: "example.test", Channel: channel, Contains: "скидка"}
	if rule.Matches(e, Message{Text: text}) {
		t.Fatal("rule hid an appointment")
	}
	if !rule.Matches(e, Message{Text: "СКИДКА на поездку"}) {
		t.Fatal("literal case-insensitive rule missed noise")
	}
	e.Package = "other.app"
	if rule.Matches(e, Message{Text: "скидка"}) {
		t.Fatal("rule crossed app boundary")
	}
	rule.Contains = "."
	if rule.Matches(e, Message{Text: "anything."}) {
		t.Fatal("overbroad rule accepted")
	}
}

func TestMessagingHistoryProjectionReplayAndBackfillIntegration(t *testing.T) {
	s, ctx, device, token := fixture(t)
	at := time.Now().UnixMilli()
	sender := "Synthetic sender"
	send := func(i int, parts []Message, change string) {
		t.Helper()
		b := testBatch()
		b.ID = fmt.Sprintf("projection_batch_%08d", i)
		b.Events[0].ID = fmt.Sprintf("projection_event_%08d", i)
		b.Events[0].Key = "same-messaging-card"
		b.Events[0].Change = change
		b.Events[0].Payload = &Payload{Messages: parts}
		ack, err := s.Accept(ctx, device, token, b)
		if err != nil || len(ack.Accepted) != 1 {
			t.Fatal(err)
		}
	}
	first := Message{Sender: &sender, Text: "First message", Timestamp: at}
	second := Message{Sender: &sender, Text: "Second message", Timestamp: at + 1}
	send(1, []Message{first}, "POSTED")
	send(2, []Message{first, second}, "UPDATED")
	send(3, []Message{first, second}, "UPDATED")
	send(4, nil, "REMOVED")
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM mobile_messages`).Scan(&n); err != nil || n != 2 {
		t.Fatal("history was counted repeatedly", n, err)
	}
	// Simulate pre-projection receipts and prove a replayable bounded upgrade.
	if _, err := s.Pool.Exec(ctx, `DELETE FROM mobile_messages; UPDATE mobile_events SET messages_indexed=false`); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexPendingMessages(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexPendingMessages(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM mobile_messages`).Scan(&n); err != nil || n != 2 {
		t.Fatal("backfill duplicated parts", n, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM mobile_events WHERE NOT messages_indexed`).Scan(&n); err != nil || n != 0 {
		t.Fatal("backfill did not checkpoint", n, err)
	}
}
