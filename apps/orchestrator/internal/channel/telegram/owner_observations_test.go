package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
)

type ownerReadFixture struct {
	calls int
	actor owneridentity.Actor
}

func (f *ownerReadFixture) OwnerOverview(ctx context.Context, id string) (task.OwnerOverview, error) {
	f.calls++
	f.actor, _ = owneridentity.FromContext(ctx)
	if f.actor.OwnerID != "123" || id != "task-owner" {
		return task.OwnerOverview{}, errors.New("PRIVATE_PROVIDER_OR_SOURCE_BODY")
	}
	return task.OwnerOverview{TaskID: id, Revision: 4, Status: "active", Attempts: []task.AttemptOverview{{RunID: "finished-attempt", TaskRevision: 3, State: "completed", AnswerState: "final", DeliveryState: "failed"}}, Actions: []task.ActionOverview{{ID: "commit-action", Tool: "http.request", Effect: "commit", EffectState: "effect_confirmed"}, {ID: "uncertain-action", Tool: "http.request", Effect: "commit", EffectState: "effect_unknown"}}, Children: []task.ChildOverview{{TaskID: "child-owner", Revision: 2, Status: "active"}}}, nil
}

type revokeFixture struct {
	actor   owneridentity.Actor
	request goal.MandateRevokeRequest
	calls   int
}

func (f *revokeFixture) RevokeMandate(ctx context.Context, r goal.MandateRevokeRequest) (goal.Receipt, error) {
	f.calls++
	f.actor, _ = owneridentity.FromContext(ctx)
	f.request = r
	return goal.Receipt{SubjectID: r.MandateID, Revision: r.ExpectedRevision + 1, Propagation: "pending"}, nil
}

func TestTelegramOwnerStateAndMandateCommandsUseTrustedActorAndFakeHTTP(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Error("owner command called provider/executor or other channel operation")
			w.WriteHeader(400)
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid channel payload")
		}
		mu.Lock()
		messages = append(messages, body.Text)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":777}}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fake-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}}, client, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, revoke := &ownerReadFixture{}, &revokeFixture{}
	a.SetOwnerTaskReader(reader)
	a.SetOwnerAuthority(revoke)
	update := Update{UpdateID: 42, Message: &Message{MessageID: 1, Chat: Chat{ID: 777}, From: &User{ID: 123}, Text: "/task task-owner"}}
	if err = a.handleMessageUpdate(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || reader.actor.OwnerID != "123" || reader.actor.Channel != "telegram" || reader.actor.SessionKey != "telegram:chat:777" || reader.actor.DeliveryID != "telegram-update-42" {
		t.Fatal("typed trusted actor lost")
	}
	mu.Lock()
	text := strings.Join(messages, "\n")
	mu.Unlock()
	for _, want := range []string{"Task result is unverified", "completed", "notification failed", "effect_confirmed", "effect_unknown", "/stop child-owner"} {
		if !strings.Contains(text, want) {
			t.Fatalf("owning state missing %q: %s", want, text)
		}
	}
	update.Message.Text = "/mandate_revoke mandate-owner 7"
	update.UpdateID = 43
	if err = a.handleMessageUpdate(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	if revoke.calls != 1 || revoke.actor.OwnerID != "123" || revoke.actor.DeliveryID != "telegram-update-43" || revoke.request.MandateID != "mandate-owner" || revoke.request.ExpectedRevision != 7 {
		t.Fatal("revocation accepted textual actor or guessed revision")
	}
	for _, invalid := range []string{"/mandate_revoke mandate-owner 0", "/mandate_revoke mandate-owner 7 owner=123", "/task task-owner owner=123"} {
		update.Message.Text = invalid
		if err = a.handleMessageUpdate(context.Background(), update); err != nil {
			t.Fatal(err)
		}
	}
	if revoke.calls != 1 || reader.calls != 1 {
		t.Fatal("ambiguous command reached owning service")
	}
	update.Message.Text = "/task task-owner"
	update.Message.From = nil
	if err = a.handleMessageUpdate(context.Background(), update); !errors.Is(err, ErrIgnoredUpdate) {
		t.Fatal("missing From actor accepted")
	}
	update.Message.From = &User{ID: 456}
	if err = a.handleMessageUpdate(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	text = strings.Join(messages, "\n")
	mu.Unlock()
	if strings.Contains(text, "PRIVATE_PROVIDER_OR_SOURCE_BODY") {
		t.Fatal("raw error/source escaped owner channel")
	}
	update.Message.Chat.ID = 999
	before := reader.calls
	if err = a.handleMessageUpdate(context.Background(), update); !errors.Is(err, ErrIgnoredUpdate) || reader.calls != before {
		t.Fatal("disallowed chat reached owner reader")
	}
}
