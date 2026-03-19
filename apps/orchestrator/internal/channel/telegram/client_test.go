package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderTelegramHTMLFormatsCommonMarkdown(t *testing.T) {
	t.Parallel()

	input := "1. **Bold** and [link](https://example.com) with `code`"
	got := renderTelegramHTML(input)
	want := "1. <b>Bold</b> and <a href=\"https://example.com\">link</a> with <code>code</code>"
	if got != want {
		t.Fatalf("renderTelegramHTML() = %q, want %q", got, want)
	}
}

func TestSendMessageUsesHTMLParseMode(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"text":"ok","chat":{"id":777}}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.SendMessage(context.Background(), 777, "**Bold**"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if payload["parse_mode"] != telegramParseModeHTML {
		t.Fatalf("parse_mode = %v, want %q", payload["parse_mode"], telegramParseModeHTML)
	}
	if payload["text"] != "<b>Bold</b>" {
		t.Fatalf("text = %v, want %q", payload["text"], "<b>Bold</b>")
	}
}

// ---------------------------------------------------------------------------
// splitMessage
// ---------------------------------------------------------------------------

func TestSplitMessageShortTextReturnsSingleChunk(t *testing.T) {
	t.Parallel()

	chunks := splitMessage("Hello world", 100)
	if len(chunks) != 1 || chunks[0] != "Hello world" {
		t.Fatalf("expected single chunk, got %+v", chunks)
	}
}

func TestSplitMessageExactLimitReturnsSingleChunk(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("a", 100)
	chunks := splitMessage(text, 100)
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("expected single chunk at exact limit, got %d chunks", len(chunks))
	}
}

func TestSplitMessageSplitsAtParagraphBoundary(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("a", 40) + "\n\n" + strings.Repeat("b", 40)
	chunks := splitMessage(text, 50)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %+v", len(chunks), chunks)
	}
	if chunks[0] != strings.Repeat("a", 40) {
		t.Fatalf("first chunk wrong: %q", chunks[0])
	}
	if chunks[1] != strings.Repeat("b", 40) {
		t.Fatalf("second chunk wrong: %q", chunks[1])
	}
}

func TestSplitMessageSplitsAtLineBoundary(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("a", 40) + "\n" + strings.Repeat("b", 40)
	chunks := splitMessage(text, 50)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %+v", len(chunks), chunks)
	}
	if chunks[0] != strings.Repeat("a", 40) {
		t.Fatalf("first chunk wrong: %q", chunks[0])
	}
}

func TestSplitMessageHardCutWhenNoBreaks(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("x", 150)
	chunks := splitMessage(text, 100)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 100 {
		t.Fatalf("first chunk len = %d, want 100", len(chunks[0]))
	}
	if len(chunks[1]) != 50 {
		t.Fatalf("second chunk len = %d, want 50", len(chunks[1]))
	}
}

func TestSplitMessageMultipleChunks(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("x", 250)
	chunks := splitMessage(text, 100)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		if len(c) > 100 {
			t.Fatalf("chunk exceeds limit: %d", len(c))
		}
		total += len(c)
	}
	if total != 250 {
		t.Fatalf("total chars = %d, want 250", total)
	}
}

func TestSplitMessageDefaultLimit(t *testing.T) {
	t.Parallel()

	// Passing 0 should use the default telegramMaxMessageLen.
	short := strings.Repeat("a", 100)
	chunks := splitMessage(short, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short text with default limit, got %d", len(chunks))
	}
}

// ---------------------------------------------------------------------------
// SendChatAction
// ---------------------------------------------------------------------------

func TestSendChatActionSendsCorrectPayload(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if err := client.SendChatAction(context.Background(), 777, ChatActionTyping); err != nil {
		t.Fatalf("SendChatAction() error = %v", err)
	}
	if payload["chat_id"] != float64(777) {
		t.Fatalf("chat_id = %v, want 777", payload["chat_id"])
	}
	if payload["action"] != ChatActionTyping {
		t.Fatalf("action = %v, want %q", payload["action"], ChatActionTyping)
	}
}

func TestSendChatActionUsesStubWhenSet(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var calledAction string
	var calledChatID int64
	client.sendChatAction = func(_ context.Context, chatID int64, action string) error {
		calledChatID = chatID
		calledAction = action
		return nil
	}

	if err := client.SendChatAction(context.Background(), 123, ChatActionUploadDocument); err != nil {
		t.Fatalf("SendChatAction() error = %v", err)
	}
	if calledChatID != 123 {
		t.Fatalf("expected chat_id 123, got %d", calledChatID)
	}
	if calledAction != ChatActionUploadDocument {
		t.Fatalf("expected action %q, got %q", ChatActionUploadDocument, calledAction)
	}
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

func TestToolDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"web_search", "web search"},
		{"file_read", "file read"},
		{"bash_exec", "bash exec"},
		{"simple", "simple"},
		{"multi_word_tool_name", "multi word tool name"},
		{"  spaced_tool  ", "spaced tool"},
		{"", ""},
	}
	for _, tc := range tests {
		got := toolDisplayName(tc.input)
		if got != tc.want {
			t.Errorf("toolDisplayName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatToolCallStarted(t *testing.T) {
	t.Parallel()

	got := formatToolCallStarted("web_search")
	if !strings.Contains(got, "web search") {
		t.Fatalf("expected tool display name in output, got %q", got)
	}
	if !strings.Contains(got, "\xF0\x9F\x94\xA7") {
		t.Fatalf("expected wrench emoji in output, got %q", got)
	}
}

func TestFormatToolCallFinishedCompleted(t *testing.T) {
	t.Parallel()

	got := formatToolCallFinished("file_read", "completed", 1500)
	if !strings.Contains(got, "file read") {
		t.Fatalf("expected tool display name, got %q", got)
	}
	if !strings.Contains(got, "completed") {
		t.Fatalf("expected 'completed' in output, got %q", got)
	}
	if !strings.Contains(got, "1.5s") {
		t.Fatalf("expected duration '1.5s', got %q", got)
	}
	if !strings.Contains(got, "\xE2\x9C\x85") {
		t.Fatalf("expected checkmark emoji, got %q", got)
	}
}

func TestFormatToolCallFinishedFailed(t *testing.T) {
	t.Parallel()

	got := formatToolCallFinished("bash_exec", "failed", 200)
	if !strings.Contains(got, "failed") {
		t.Fatalf("expected 'failed' in output, got %q", got)
	}
	if !strings.Contains(got, "\xE2\x9D\x8C") {
		t.Fatalf("expected X emoji, got %q", got)
	}
}

func TestFormatToolCallFinishedNoDuration(t *testing.T) {
	t.Parallel()

	got := formatToolCallFinished("test_tool", "completed", 0)
	if strings.Contains(got, "s)") {
		t.Fatalf("expected no duration when durationMs=0, got %q", got)
	}
	if !strings.Contains(got, "completed") {
		t.Fatalf("expected 'completed' in output, got %q", got)
	}
}
