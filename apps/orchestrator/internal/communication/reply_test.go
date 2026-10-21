package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

func TestCommunicationCrossChannelReplyCanonicalTargetIntegration(t *testing.T) {
	for _, withTask := range []bool{false, true} {
		t.Run(map[bool]string{false: "conversation", true: "task"}[withTask], func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: withTask})
			p, e := f.publish(t, "result", "communication.message", message)
			if e != nil {
				t.Fatal(e)
			}
			d := &communication.DeliveryStore{Core: f.s}
			for i := 0; i < 2; i++ {
				m, e := d.Claim(f.ctx, 30e9)
				if e != nil || m == nil {
					t.Fatal(m, e)
				}
				if e = d.AuthorizeSend(f.ctx, *m); e != nil {
					t.Fatal(e)
				}
				if e = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: m.Channel + ":chat:42"}); e != nil {
					t.Fatal(e)
				}
			}
			q := communication.ReplyRequest{BindingID: f.bindings["telegram"], BindingVersion: 1, ChannelACK: "telegram:chat:42", Text: "Revise the second point"}
			r, e := f.s.Reply(f.actor("telegram", "ordinary-reply"), q)
			if e != nil || r.SessionKey != f.p.SessionKey || r.TaskID != f.p.TaskID {
				t.Fatal(r, e)
			}
			again, e := f.s.Reply(f.actor("telegram", "ordinary-reply"), q)
			if e != nil || again.InputID != r.InputID {
				t.Fatal("duplicate ingress", again, e)
			}
			if strings.Contains(r.Event.PayloadJSON, "grant") || !strings.Contains(r.Event.PayloadJSON, p.PublicationRef) {
				t.Fatal("reply gained authority or lost provenance")
			}
			q.ChannelACK = ""
			if _, e = f.s.Reply(f.actor("telegram", "no-target"), q); !errors.Is(e, communication.ErrAmbiguous) {
				t.Fatal("guessed latest task", e)
			}
			q.PublicationRef = p.PublicationRef
			q.BindingID = f.bindings["web"]
			if _, e = f.s.Reply(f.actor("web", "web-reply"), q); e != nil {
				t.Fatal("cross-channel Web target", e)
			}
			f.revoked.Store(true)
			if _, e = f.s.Reply(f.actor("web", "revoked-reply"), q); e == nil {
				t.Fatal("revoked source reply")
			}
		})
	}
}

func TestCommunicationReplyProofAndBindingRevocationIntegration(t *testing.T) {
	for _, scenario := range []string{"wrong_owner", "wrong_recipient", "rebind", "disable", "stale_task", "widget", "unconfirmed_ack"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: true})
			method, raw := "communication.message", message
			if scenario == "widget" {
				method, raw = "communication.widget", widget
			}
			p, e := f.publish(t, "one", method, raw)
			if e != nil {
				t.Fatal(e)
			}
			ctx := f.actor("telegram", "reply")
			d := &communication.DeliveryStore{Core: f.s}
			for i := 0; i < 2; i++ {
				m, e := d.Claim(f.ctx, 30e9)
				if e != nil || m == nil || m.PublicationID != p.PublicationRef {
					t.Fatal(m, e)
				}
				if e = d.AuthorizeSend(f.ctx, *m); e != nil {
					t.Fatal(e)
				}
				if e = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: m.Channel + ":proof"}); e != nil {
					t.Fatal(e)
				}
			}
			q := communication.ReplyRequest{BindingID: f.bindings["telegram"], BindingVersion: 1, ChannelACK: "telegram:proof", Text: "continue"}
			switch scenario {
			case "wrong_owner":
				ctx = owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "other", Channel: "telegram", SessionKey: "telegram:verified", DeliveryID: "forged"})
			case "wrong_recipient":
				ctx = owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: "telegram:other", DeliveryID: "forged"})
			case "rebind", "disable":
				recipient := "telegram:verified"
				if scenario == "rebind" {
					recipient = "telegram:other"
				}
				actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: recipient, DeliveryID: "update"})
				e = f.s.Bind(actor, communication.Binding{ID: q.BindingID, Channel: "telegram", Recipient: recipient, Enabled: scenario != "disable", Capabilities: []string{"message", "widget"}}, 1)
			case "stale_task":
				_, e = f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.p.TaskID)
			case "unconfirmed_ack":
				q.ChannelACK = "telegram:chat:42"
			}
			if e != nil {
				t.Fatal(e)
			}
			before := f.count(t, "accepted_inputs")
			if _, e = f.s.Reply(ctx, q); e == nil {
				t.Fatal("unsafe reply accepted")
			}
			if f.count(t, "accepted_inputs") != before {
				t.Fatal("denial created input")
			}
		})
	}
}

func TestCommunicationOwnerHTTPProofAndStrictBodiesIntegration(t *testing.T) {
	f := setup(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "owner-http-deletions"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	if e := sources.PinTranscript(f.ctx, f.p.SessionKey, 0); e != nil {
		t.Fatal(e)
	}
	f.s.SourceGuard = sources.CheckRunSourcesTx
	token := strings.Repeat("owner", 9)
	h := api.CommunicationOwnerControls(api.NewOwnerControlAuth(token, f.p.OwnerID), f.s)
	request := func(path, body, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+auth)
		r.Header.Set("X-Butler-Delivery-ID", "stable-request")
		r.Header.Set("X-Owner-ID", f.p.OwnerID)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"binding_id":"web-api","expected_version":0,"enabled":true,"capabilities":["message","widget"]}`
	if w := request("/api/v3/channel-bindings", body, "wrong"); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("/api/v3/channel-bindings", "null", token); w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("/api/v3/channel-bindings", strings.TrimSuffix(body, "}")+`,"owner_id":"forged"}`, token); w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("/api/v3/channel-bindings", body, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM communication_bindings WHERE binding_id='web-api'`)
	})
	p, e := f.publish(t, "http-result", "communication.message", message)
	if e != nil {
		t.Fatal(e)
	}
	q, _ := json.Marshal(communication.ReplyRequest{BindingID: "web-api", BindingVersion: 1, PublicationRef: p.PublicationRef, Text: "revise"})
	if w := request("/api/v3/communications/replies", string(q), token); w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("/api/v3/communications/replies", string(q), token); w.Code != 202 {
		t.Fatal("receipt replay", w.Code, w.Body)
	}
	if w := request("/api/v3/communications/replies", strings.Replace(string(q), "revise", "changed", 1), token); w.Code != 409 {
		t.Fatal("conflicting delivery replay", w.Code, w.Body)
	}
	if w := request("/api/v3/communications/replies", `{"text":"continue"}`, token); w.Code != 409 || !strings.Contains(w.Body.String(), "explicit_target_required") {
		t.Fatal("unproved reply was guessed", w.Code, w.Body)
	}
	question, e := f.publish(t, "http-question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	responseBody := `{"binding_id":"web-api","binding_version":1,"expected_version":1,"answer":{"option_ids":["a"]}}`
	if w := request("/api/v3/interactions/"+question.InteractionRef+"/responses", responseBody, token); w.Code != 200 {
		t.Fatal("owner response blocked by FIFO", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", "/api/v3/channel-bindings", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "recipient") || strings.Contains(w.Body.String(), token) {
		t.Fatal(w.Code, w.Body)
	}
}

func TestCommunicationACKAmbiguityNeverChoosesLatestPublicationIntegration(t *testing.T) {
	f := setup(t)
	for _, call := range []string{"one", "two"} {
		if _, e := f.publish(t, call, "communication.message", message); e != nil {
			t.Fatal(e)
		}
	}
	d := &communication.DeliveryStore{Core: f.s}
	for i := 0; i < 4; i++ {
		m, e := d.Claim(f.ctx, 30e9)
		if e != nil || m == nil {
			t.Fatal(m, e)
		}
		if e = d.AuthorizeSend(f.ctx, *m); e != nil {
			t.Fatal(e)
		}
		if e = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: m.Channel + ":collision"}); e != nil {
			t.Fatal(e)
		}
	}
	_, e := f.s.Reply(f.actor("telegram", "reply"), communication.ReplyRequest{BindingID: f.bindings["telegram"], BindingVersion: 1, ChannelACK: "telegram:collision", Text: "continue"})
	if !errors.Is(e, communication.ErrAmbiguous) {
		t.Fatal("ambiguous ACK guessed a publication", e)
	}
	if f.count(t, "accepted_inputs") != 1 {
		t.Fatal("ambiguity enqueued work")
	}
}
