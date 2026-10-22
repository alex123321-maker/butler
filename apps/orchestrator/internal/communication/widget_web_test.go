package communication_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func TestCommunicationWidgetActualWebControlAndFeedIntegration(t *testing.T) {
	f := setup(t)
	f.s.WebAvailable = true
	b := communication.Binding{ID: f.bindings["web"], Version: 2, Channel: "web", Recipient: "web:" + f.p.OwnerID, Enabled: true, Capabilities: []string{"message", "widget"}}
	actor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: b.Recipient, DeliveryID: "bind"})
	if e := f.s.Bind(actor, b, 1); e != nil {
		t.Fatal(e)
	}
	w := &communication.WebReader{Core: f.s, Binding: b, Resolve: func(context.Context, pgx.Tx, string, communication.Binding, string) (artifacts.Media, error) {
		return artifacts.Media{}, communication.ErrDenied
	}}
	f.s.PlanDelivery = func(ctx context.Context, tx pgx.Tx, run string, binding communication.Binding, a communicationcontrol.Args) ([]string, error) {
		if binding.Channel == "web" {
			return w.Plan(ctx, tx, run, binding, a)
		}
		return []string{a.Question}, nil
	}
	pub, e := f.publish(t, "widget", "communication.widget", multiWidget)
	if e != nil {
		t.Fatal(e)
	}
	handler := api.CommunicationWeb(api.NewOwnerControlAuth("synthetic-owner-proof-12345678901234567890", f.p.OwnerID), w, nil, nil)
	q := communication.WidgetAction{BindingID: b.ID, BindingVersion: 2, ExpectedVersion: 1, DraftVersion: 1, Operation: "draft", Answer: communicationcontrol.Answer{OptionIDs: []string{"a"}}}
	post := func(path, proof string, q communication.WidgetAction) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(q)
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
		request.Header.Set("Authorization", "Bearer "+proof)
		request.Header.Set("X-Butler-Delivery-ID", "web-control-"+q.Operation)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		return result
	}
	path := "/api/v3/interactions/" + url.PathEscape(pub.InteractionRef) + "/actions"
	// Match the exact encoding used by browser encodeURIComponent.
	path = strings.ReplaceAll(path, ":", "%3A")
	if r := post(path, "wrong", q); r.Code != 401 {
		t.Fatal("owner guard", r.Code)
	}
	if r := post(path, "synthetic-owner-proof-12345678901234567890", q); r.Code != 200 {
		t.Fatal("real control route", r.Code, r.Body.String())
	}
	page, e := w.Feed(actor, 0, 50)
	if e != nil || len(page.Items) != 1 || page.Items[0].Interaction == nil || page.Items[0].Interaction.DraftVersion != 2 || page.Items[0].Interaction.State != "open" {
		t.Fatal("shared draft absent from feed", page, e)
	}
	q.Operation = "submit"
	q.DraftVersion = 2
	q.Answer = communicationcontrol.Answer{}
	if r := post(path, "synthetic-owner-proof-12345678901234567890", q); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := post(path, "synthetic-owner-proof-12345678901234567890", q); r.Code != 200 {
		t.Fatal("lost ACK replay", r.Code, r.Body.String())
	}
	page, e = w.Feed(actor, 0, 50)
	if e != nil || page.Items[0].Interaction.State != "resolved" {
		t.Fatal(page, e)
	}
	if len(page.Messages) != 0 {
		t.Fatal("internal response JSON duplicated into conversation", page.Messages)
	}
	for _, suffix := range []string{"?x=1", "%2F", "%252F"} {
		if r := post(path+suffix, "synthetic-owner-proof-12345678901234567890", q); r.Code == 200 {
			t.Fatal("noncanonical path accepted", suffix)
		}
	}
}
