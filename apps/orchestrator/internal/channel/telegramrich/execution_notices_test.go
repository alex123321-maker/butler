package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

func TestExecutionNoticeSenderRequiresExactBotAndActualACK(t *testing.T) {
	for _, tc := range []struct {
		name, response, want string
		bot                  int64
	}{
		{"ack", `{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`, "sent", 99},
		{"wrong_chat", `{"ok":true,"result":{"message_id":7,"chat":{"id":43}}}`, "delivery_unknown", 99},
		{"malformed", `{"ok":true,"result":{}}`, "delivery_unknown", 99},
		{"wrong_bot", `{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`, "absent", 98},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends, guards := 0, 0
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "getMe") {
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": tc.bot, "is_bot": true}})
					return
				}
				sends++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["text"] != "Обрабатываю запрос." {
					t.Error("unexpected notice content")
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer site.Close()
			sender := &ExecutionSender{Client: &Client{token: "synthetic", base: site.URL, http: site.Client()}, Binding: OwnerBinding{BindingID: "bot", Version: 2, ChatID: 42, BotID: 99}, Authorize: func(context.Context, deliveryoutbox.Message) error { guards++; return nil }}
			out := sender.SendStored(context.Background(), deliveryoutbox.Message{NotificationID: "execution:test", ResultID: "execution:test", BindingID: "bot", BindingVersion: 2, Recipient: "telegram:42", Channel: "telegram", Content: "Обрабатываю запрос."})
			if out.Status != tc.want {
				t.Fatalf("got %+v want %s", out, tc.want)
			}
			if tc.name == "wrong_bot" && sends != 0 {
				t.Fatal("wrong bot sent notice")
			}
			if tc.name != "wrong_bot" && (sends != 1 || guards != 1) {
				t.Fatal("send/guard mismatch", sends, guards)
			}
		})
	}
}
