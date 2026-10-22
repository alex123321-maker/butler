package app

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationDrainRetainsAcceptedIngressAndRejectsNewInputsIntegration(t *testing.T) {
	f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions"))
	inputs := input.NewStore(f.pool)
	store := &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: f.sources.CheckRunSourcesTx}
	b := communication.Binding{ID: "drain-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	actor := owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", SessionKey: b.Recipient, DeliveryID: "telegram-rich:77:1"}
	tg := owneridentity.WithActor(f.ctx, actor)
	if err := store.Bind(tg, b, 0); err != nil {
		t.Fatal(err)
	}
	policy := telegramExportPolicy{owner: f.parent.OwnerID, binding: b, allowPrivate: true, catalog: f.sources, sourceGuard: f.sources.CheckRunSourcesTx}
	store.ChannelGuard = policy.check
	store.PlanDelivery = (telegramrich.Planner{Profile: telegramrich.Capabilities{Message: true, Photo: true, Rich: true, Document: true}, Resolve: policy.media}).Plan
	c := &telegramCommunication{store: store, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: b.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: policy}
	c.installInputHooks()
	if err := c.installWeb(f.ctx, TelegramCommunicationOptions{OwnerID: f.parent.OwnerID, WebBindingID: "drain-web"}, f.sources.CheckRunSourcesTx); err != nil {
		t.Fatal(err)
	}
	update := telegramrich.Update{ID: 1, Message: &telegramrich.Message{Text: "Retained Telegram request"}}
	if err := c.conversation(tg, update); err != nil {
		t.Fatal(err)
	}
	webActor := owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: c.web.binding.Recipient, DeliveryID: "web-first"}
	web := owneridentity.WithActor(f.ctx, webActor)
	first, err := c.webConversation(web, "Retained Web request")
	if err != nil {
		t.Fatal(err)
	}
	c.draining = true
	if err = c.conversation(tg, update); err != nil {
		t.Fatal("lost cursor ACK replay was blocked by drain", err)
	}
	again, err := c.webConversation(web, "Retained Web request")
	if err != nil || again.InputID != first.InputID {
		t.Fatal("lost Web ACK replay changed identity", err)
	}
	actor.DeliveryID = "telegram-rich:77:2"
	if err = c.conversation(owneridentity.WithActor(f.ctx, actor), update); !errors.Is(err, communication.ErrDraining) {
		t.Fatal("new Telegram request accepted during drain", err)
	}
	webActor.DeliveryID = "web-second"
	if _, err = c.webConversation(owneridentity.WithActor(f.ctx, webActor), "New Web request"); !errors.Is(err, communication.ErrDraining) {
		t.Fatal("new Web request accepted during drain", err)
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs`).Scan(&count); err != nil || count != 2 {
		t.Fatal("drain partially committed or duplicated input", count, err)
	}
	if _, err = inputs.Claim(f.ctx, time.Minute); !errors.Is(err, input.ErrNotFound) {
		t.Fatal("old worker claimed epoch1", err)
	}
	inputs.CommunicationEpoch = 1
	if _, err = inputs.Claim(f.ctx, time.Minute); err != nil {
		t.Fatal("drain prevented an already accepted input from running", err)
	}
	token := strings.Repeat("d", 40)
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(token, f.parent.OwnerID), c.web.reader, nil, c.webConversation, true)
	q := httptest.NewRequest("GET", "/api/v3/communications/profile", nil)
	q.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, q)
	var fields map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &fields) != nil || fields["ready"] != true || fields["drain_only"] != true {
		t.Fatal("drain disabled read readiness", rec.Code, fields)
	}
}
