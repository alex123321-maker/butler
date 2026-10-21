package telegramrich

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestCommunicationTransportNoticeOutcomeAndCommitGapIntegration(t *testing.T) {
	for _, scenario := range []string{"sent", "unknown", "lost-finish", "sending-before-network", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `INSERT INTO communication_bindings(binding_id,owner_id,channel,recipient,version,enabled,verified,capabilities) VALUES('bot','owner','telegram','telegram:42',1,TRUE,TRUE,'["message"]')`); err != nil {
				t.Fatal(err)
			}
			var sent atomic.Int32
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/getMe") {
					_, _ = w.Write([]byte(`{"ok":true,"result":{"id":77,"is_bot":true}}`))
					return
				}
				sent.Add(1)
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["text"] != noticeText("target_required") || body["protect_content"] != true {
					t.Error("notice accepted arbitrary content")
				}
				if scenario == "unknown" {
					_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
					return
				}
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
			})
			b := OwnerBinding{OwnerID: "owner", BindingID: "bot", Version: 1, BotID: 77, ChatID: 42, UserID: 42}
			n := &Notices{Pool: pool, Client: c, Binding: b}
			u := Update{ID: 5, Message: &Message{ID: 10, Text: "private owner text must never enter a transport receipt", Chat: Chat{ID: 42, Type: "private"}, From: User{ID: 42}}}
			if scenario == "lost-finish" {
				if _, err := pool.Exec(ctx, `CREATE FUNCTION lose_notice_finish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='sent' THEN RAISE EXCEPTION 'notice finish lost'; END IF; RETURN NEW; END $$; CREATE TRIGGER lose_notice_finish BEFORE UPDATE ON communication_telegram_notices FOR EACH ROW EXECUTE FUNCTION lose_notice_finish()`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "sending-before-network" {
				if _, err := pool.Exec(ctx, `INSERT INTO communication_telegram_notices VALUES(77,5,'bot',1,'target_required','sending','',clock_timestamp())`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "revoked" {
				if _, err := pool.Exec(ctx, `UPDATE communication_bindings SET enabled=FALSE`); err != nil {
					t.Fatal(err)
				}
			}
			err := n.Notify(ctx, u, "target_required")
			if scenario == "revoked" {
				if err == nil || sent.Load() != 0 {
					t.Fatal("revoked notice sent", err, sent.Load())
				}
				return
			}
			if scenario == "lost-finish" {
				if err == nil {
					t.Fatal("missing finish failure")
				}
				if _, err = pool.Exec(ctx, `DROP TRIGGER lose_notice_finish ON communication_telegram_notices`); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			n = &Notices{Pool: pool, Client: c, Binding: b}
			if err = n.Notify(ctx, u, "target_required"); err != nil {
				t.Fatal(err)
			}
			var state, ack string
			if err = pool.QueryRow(ctx, `SELECT status,channel_ack FROM communication_telegram_notices WHERE bot_id=77 AND update_id=5`).Scan(&state, &ack); err != nil {
				t.Fatal(err)
			}
			want := int32(1)
			if scenario == "sending-before-network" {
				want = 0
			}
			if sent.Load() != want {
				t.Fatal("transport notice repeated after recovery", sent.Load())
			}
			if scenario == "sent" {
				if state != "sent" || ack != "7" {
					t.Fatal(state, ack)
				}
			} else if state != "delivery_unknown" {
				t.Fatal("uncertain notice became success", state)
			}
		})
	}
}
