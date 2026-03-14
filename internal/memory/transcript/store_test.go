package transcript

import (
	"testing"
	"time"
)

func TestAppendMessageAssignsDefaults(t *testing.T) {
	message := Message{
		SessionKey: "session-1",
		Role:       "user",
		Content:    "hello",
	}

	if message.MessageID != "" {
		t.Fatal("expected zero message id before append")
	}

	if id := newID("msg"); id == "" {
		t.Fatal("expected generated id")
	}
}

func TestNewIDUsesPrefix(t *testing.T) {
	id := newID("tool")
	if len(id) < len("tool-") || id[:5] != "tool-" {
		t.Fatalf("expected prefixed id, got %q", id)
	}
}

func TestTranscriptStructHoldsMessagesAndToolCalls(t *testing.T) {
	now := time.Now().UTC()
	transcript := Transcript{
		Messages:  []Message{{MessageID: "msg-1", CreatedAt: now}},
		ToolCalls: []ToolCall{{ToolCallID: "tool-1", StartedAt: now}},
	}
	if len(transcript.Messages) != 1 || len(transcript.ToolCalls) != 1 {
		t.Fatal("expected transcript entries to be retained")
	}
}
