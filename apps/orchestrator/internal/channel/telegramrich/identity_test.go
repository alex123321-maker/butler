package telegramrich

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

func TestCommunicationPinnedIdentityBeforeEveryUnverifiedOperation(t *testing.T) {
	for _, mode := range []string{"outage", "wrong_bot", "recovered", "revoked_during_identity"} {
		t.Run(mode, func(t *testing.T) {
			var probes, sends atomic.Int32
			var available, allowed atomic.Bool
			allowed.Store(true)
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/getMe") {
					probes.Add(1)
					if !available.Load() {
						w.WriteHeader(503)
						_, _ = w.Write([]byte(`{"ok":false}`))
						return
					}
					id := 77
					if mode == "wrong_bot" {
						id = 88
					}
					if mode == "revoked_during_identity" {
						allowed.Store(false)
					}
					_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"id":%d,"is_bot":true,"username":"fixture_bot"}}`, id)
					return
				}
				sends.Add(1)
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
			})
			if c.RequireIdentity("fixture_bot", 77) != nil {
				t.Fatal("identity pin failed")
			}
			if !c.IdentityVerificationPending() {
				t.Fatal("unverified identity reported ready")
			}
			if _, _, err := c.call(context.Background(), "getUpdates", []byte(`{}`), "application/json"); err == nil || sends.Load() != 0 || probes.Load() != 0 {
				t.Fatal("unverified client released a non-identity operation")
			}
			s := Sender{Client: c, Authorize: func(context.Context, deliveryoutbox.Message) error {
				if !allowed.Load() {
					return ErrUnavailable
				}
				return nil
			}}
			if out := s.SendStored(context.Background(), stored(textPart())); out.Status != "absent" || !out.Retryable || sends.Load() != 0 {
				t.Fatal("unverified send escaped or was called unknown")
			}
			if mode == "outage" {
				if probes.Load() != 1 {
					t.Fatal("unexpected identity retries")
				}
				return
			}
			available.Store(true)
			out := s.SendStored(context.Background(), stored(textPart()))
			if mode == "recovered" {
				if c.IdentityVerificationPending() {
					t.Fatal("verified identity remained pending")
				}
				if out.Status != "sent" || sends.Load() != 1 {
					t.Fatalf("verified recovery failed: status=%s code=%s probes=%d sends=%d verified=%v", out.Status, out.ErrorCode, probes.Load(), sends.Load(), c.identityVerified.Load())
				}
				if out = s.SendStored(context.Background(), stored(textPart())); out.Status != "sent" || probes.Load() != 2 {
					t.Fatal("immutable client identity was not cached")
				}
			} else if out.Status != "absent" || sends.Load() != 0 {
				t.Fatal("changed identity/authority released message")
			}
		})
	}
}
