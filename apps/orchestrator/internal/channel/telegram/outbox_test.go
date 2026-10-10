package telegram

import (
	"context"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoredSendAcknowledgementAndSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		retry            bool
	}{
		{"accepted", `{"ok":true,"result":{"message_id":123,"chat":{"id":42}}}`, "sent", 200, false},
		{"rejected", `{"ok":false,"description":"private token and body must disappear","error_code":429,"parameters":{"retry_after":2}}`, "absent", 429, true},
		{"permanent", `{"ok":false,"description":"secret","error_code":403}`, "absent", 403, false},
		{"wrong recipient", `{"ok":true,"result":{"message_id":123,"chat":{"id":999}}}`, "delivery_unknown", 200, false},
		{"malformed", `private raw response`, "delivery_unknown", 502, false},
		{"missing ack", `{"ok":true,"result":{}}`, "delivery_unknown", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var n atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "private-secret-token", server.Client())
			outcome := client.sendStoredPlain(context.Background(), 42, "saved notification")
			if outcome.Status != tc.want || outcome.Retryable != tc.retry || n.Load() != 1 {
				t.Fatalf("wrong single attempt: %#v requests=%d", outcome, n.Load())
			}
			if strings.Contains(fmt.Sprintf("%#v", outcome), "private") || strings.Contains(fmt.Sprintf("%#v", outcome), "secret") {
				t.Fatal("credential/body leaked")
			}
		})
	}
}
func TestStoredSendTimeoutAndRedirectNeverRetry(t *testing.T) {
	for _, mode := range []string{"timeout", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var n atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n.Add(1)
				if mode == "timeout" {
					time.Sleep(100 * time.Millisecond)
					return
				}
				http.Redirect(w, r, "/would-repeat", 307)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "private-secret-token", server.Client())
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			out := client.sendStoredPlain(ctx, 42, "saved notification")
			if out.Status != "delivery_unknown" || out.Retryable || n.Load() != 1 {
				t.Fatalf("ambiguous send retried: %#v %d", out, n.Load())
			}
		})
	}
}
func TestStoredSenderUsesTrustedSessionRecipient(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":9,"chat":{"id":42}}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "fixture", server.Client())
	adapter, _ := NewAdapter(Config{AllowedChatIDs: []string{"42"}}, client, nil, nil)
	for _, key := range []string{"telegram:chat:99", "web:42", "garbage"} {
		out := adapter.SendStored(context.Background(), deliveryoutbox.Message{SessionKey: key, Channel: "telegram", Content: "saved"})
		if out.Status != "absent" || out.Retryable {
			t.Fatal("untrusted destination accepted")
		}
	}
	if calls != 0 {
		t.Fatal("wrong owner/channel caused network request")
	}
	out := adapter.SendStored(context.Background(), deliveryoutbox.Message{SessionKey: "telegram:chat:42", Channel: "telegram", Content: "saved"})
	if out.Status != "sent" || calls != 1 {
		t.Fatal(out)
	}
}
