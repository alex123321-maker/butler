package communication_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func webFixture(t *testing.T) (*fixture, *communication.WebReader, string, string) {
	t.Helper()
	f, image := mediaFixture(t)
	f.s.WebAvailable = true
	document, err := artifacts.NewService(artifacts.NewPostgresRepository(f.pool)).SaveMedia(f.ctx, f.p.RunID, f.p.SessionKey, "проверка.txt", "text/plain", []byte("Unicode 🧪\nRetained document\n"))
	if err != nil {
		t.Fatal(err)
	}
	b := communication.Binding{ID: f.bindings["web"], Version: 2, Channel: "web", Recipient: "web:verified", Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	if err = f.s.Bind(f.actor("web", "web-test-binding"), b, 1); err != nil {
		t.Fatal(err)
	}
	w := &communication.WebReader{Core: f.s, Binding: b, Resolve: func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, ref string) (artifacts.Media, error) {
		return artifacts.ResolveMediaTx(ctx, tx, run, ref, func(ctx context.Context, tx pgx.Tx, run, ref string) error { return f.s.SourceGuard(ctx, tx, run) })
	}}
	planner := f.s.PlanDelivery
	f.s.PlanDelivery = func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, a communicationcontrol.Args) ([]string, error) {
		if b.Channel == "web" {
			return w.Plan(ctx, tx, run, b, a)
		}
		return planner(ctx, tx, run, b, a)
	}
	return f, w, image, document
}

func webReport(image, document string) string {
	raw, _ := json.Marshal(communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Title: "Полный отчёт 🧪", Summary: "<script>synthetic()</script>", Blocks: []communicationcontrol.Block{
		{Type: "heading", Text: "Заголовок"}, {Type: "paragraph", Text: "Полный текст"}, {Type: "quote", Text: "Цитата"}, {Type: "code", Text: "<img onerror=synthetic()>"},
		{Type: "list", Items: []string{"Один", "Два 🧪"}}, {Type: "table", Rows: [][]string{{"Компонент", "Результат"}, {"Core", "Подключён"}, {"Broker", "Проверен"}}},
		{Type: "divider"}, {Type: "image", ArtifactRef: image, Alt: "Изображение 🧪"}, {Type: "document", ArtifactRef: document, Alt: "Документ"},
	}, Attachments: []string{document}})
	return string(raw)
}

func TestCommunicationWebCanonicalReportAndIndependentDeliveryIntegration(t *testing.T) {
	f, w, image, document := webFixture(t)
	r, err := f.publish(t, "web-report", "communication.report", webReport(image, document))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Delivery) != 2 {
		t.Fatal("missing independent targets", r)
	}
	page, err := w.Feed(f.actor("web", "read"), 0, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatal("feed", page, err)
	}
	p := page.Items[0]
	if !p.Available || p.Ref != r.PublicationRef || p.Payload.Title != "Полный отчёт 🧪" || len(p.Payload.Blocks) != 9 || len(p.Files) != 2 || len(p.Parts) != 2 {
		t.Fatal("canonical report lost", p)
	}
	for _, part := range p.Parts {
		if part.Channel == "web" && (part.State != "available" || part.ACK != "") {
			t.Fatal("Web read became send/read receipt", part)
		}
	}
	m, err := w.Media(f.actor("web", "download"), p.Ref, document)
	if err != nil || string(m.Data) != "Unicode 🧪\nRetained document\n" {
		t.Fatal("full retained document", err)
	}
	if _, err = w.Media(f.actor("web", "arbitrary"), p.Ref, "artifact:unrelated"); err == nil {
		t.Fatal("arbitrary artifact escaped publication scope")
	}
	// Telegram failure/UNKNOWN cannot hide a permitted Web document or rerun it.
	d := (&communication.DeliveryStore{Core: f.s}).ForBinding(communication.Binding{ID: f.bindings["telegram"], Version: 2, Channel: "telegram", Recipient: "telegram:42"})
	part, err := d.Claim(f.ctx, 30*time.Second)
	if err != nil || part == nil {
		t.Fatal(part, err)
	}
	if err = d.Finish(f.ctx, *part, deliveryoutbox.Outcome{Status: "delivery_unknown"}); err != nil {
		t.Fatal(err)
	}
	p, err = w.Detail(f.actor("web", "after-restart"), p.Ref)
	if err != nil || !p.Available {
		t.Fatal("Telegram outcome affected Web availability", err)
	}
	if f.count(t, "messages") != 1 || f.count(t, "communication_publications") != 1 {
		t.Fatal("channel read duplicated canonical history")
	}
	page, err = w.Feed(f.actor("web", "resume"), page.Cursor, 50)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("feed cursor replay", page, err)
	}
	if _, err = d.Claim(f.ctx, 30*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestCommunicationWebOwnerBindingSourceAndMediaRevocationIntegration(t *testing.T) {
	for _, kind := range []string{"wrong-owner", "binding-disabled", "binding-rebound", "source-revoked", "media-redacted"} {
		t.Run(kind, func(t *testing.T) {
			f, w, image, document := webFixture(t)
			r, err := f.publish(t, "web-report", "communication.report", webReport(image, document))
			if err != nil {
				t.Fatal(err)
			}
			ctx := f.actor("web", "read")
			switch kind {
			case "wrong-owner":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "someone-else", Channel: "web", SessionKey: "web:verified", DeliveryID: "read"})
			case "binding-disabled":
				_, err = f.pool.Exec(ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, w.Binding.ID)
			case "binding-rebound":
				_, err = f.pool.Exec(ctx, `UPDATE communication_bindings SET version=version+1 WHERE binding_id=$1`, w.Binding.ID)
			case "source-revoked":
				f.revoked.Store(true)
			case "media-redacted":
				_, err = f.pool.Exec(ctx, `UPDATE artifacts SET privacy_redacted=TRUE WHERE artifact_id=$1`, strings.TrimPrefix(document, "artifact:"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Detail(ctx, r.PublicationRef); err == nil {
				t.Fatal("stale detail allowed", kind)
			}
			if _, err = w.Media(ctx, r.PublicationRef, image); err == nil {
				t.Fatal("stale media allowed", kind)
			}
			page, err := w.Feed(ctx, 0, 50)
			if err == nil {
				for _, p := range page.Items {
					if p.Available || p.Payload != nil || len(p.Files) > 0 || len(p.Parts) > 0 {
						t.Fatal("stale feed payload retained", kind)
					}
				}
			}
		})
	}
}

func TestCommunicationWebHTTPAuthScopedSSEAndMediaIntegration(t *testing.T) {
	f, w, image, document := webFixture(t)
	r, err := f.publish(t, "web-report", "communication.report", webReport(image, document))
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("s", 40)
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(token, f.p.OwnerID), w, nil, nil)
	for _, tc := range []struct {
		path, proof string
		status      int
	}{
		{"/api/v3/communications", "", 401}, {"/api/v3/communications?cursor=-1", token, 400}, {"/api/v3/communications?cursor=0&cursor=1", token, 400},
		{"/api/v3/communications?owner_id=someone-else", token, 400}, {"/api/v3/communications/profile", token, 200},
		{"/api/v3/communications/" + r.PublicationRef + "/media?artifact_ref=" + document, token, 200},
	} {
		q := httptest.NewRequest("GET", tc.path, nil)
		if tc.proof != "" {
			q.Header.Set("Authorization", "Bearer "+tc.proof)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, q)
		if rec.Code != tc.status {
			t.Fatal(tc.path, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response cached")
		}
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v3/communications/events", nil)
	q.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(q)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	buffer := bufio.NewReader(response.Body)
	var frame strings.Builder
	for {
		line, e := buffer.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		frame.WriteString(line)
		if line == "\n" {
			break
		}
	}
	if response.StatusCode != 200 || !strings.Contains(frame.String(), "event: refresh") || strings.Contains(frame.String(), "Полный отчёт") || strings.Contains(frame.String(), "artifact:") {
		t.Fatal("SSE must carry scoped invalidation only", response.StatusCode, frame.String())
	}
	cancel()
}
