package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
)

type releaseChainChannel struct {
	sender                    *telegram.Adapter
	mu                        sync.Mutex
	expected, fault           string
	total, requests, accepted atomic.Int32
}

// One actual Telegram adapter and one synthetic recipient span ordinary Core
// progress and final dispatch. Any technical request is observed and rejected.
func newReleaseChainChannel(t *testing.T) *releaseChainChannel {
	t.Helper()
	c := &releaseChainChannel{}
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.total.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/botrelease-fixture/sendMessage" {
			http.Error(w, "synthetic unexpected technical route", 400)
			return
		}
		var q struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		d := json.NewDecoder(io.LimitReader(r.Body, 4096))
		d.DisallowUnknownFields()
		c.mu.Lock()
		body, fault := c.expected, c.fault
		c.mu.Unlock()
		if d.Decode(&q) != nil || d.Decode(new(any)) != io.EOF || body == "" || q.ChatID != 424242 || q.Text != body {
			http.Error(w, "one frozen task notice required", 400)
			return
		}
		n := c.requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if fault == "explicit_absence" && n == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"ok":false,"error_code":429,"description":"synthetic explicit absence","parameters":{"retry_after":1}}`)
			return
		}
		c.accepted.Add(1)
		io.WriteString(w, `{"ok":true,"result":{"message_id":101,"chat":{"id":424242,"type":"private"}}}`)
	}))
	t.Cleanup(bot.Close)
	client, e := telegram.NewClient(bot.URL, "release-fixture", bot.Client())
	if e != nil {
		t.Fatal(e)
	}
	c.sender, e = telegram.NewAdapter(telegram.Config{AllowedChatIDs: []string{"424242"}}, client, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
