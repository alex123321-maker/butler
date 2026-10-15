package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestTemporalOwnerAPIStrictDatesCASAndPrivateHistoricalReadIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	s := &MemoryCatalogServer{Store: catalog.New(pool, m), Auth: NewOwnerControlAuth(strings.Repeat("t", 40), "owner")}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "seed"})
	src, err := s.Store.Put(ctx, catalog.PutSource{ExternalKey: "temporal", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Пиши по-русски"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Store.Propose(ctx, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: "language", Value: json.RawMessage(`"ru"`), Summary: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Store.Decide(ctx, catalog.Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 40))
		}
		r.Header.Set("X-Butler-Delivery-ID", "temporal")
		w := httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		return w
	}
	path := "/api/v4/memory/facts?claim_id=" + c.ID
	if w := request("GET", path, "", true); w.Code != 503 {
		t.Fatal("default-off route admitted", w.Code)
	}
	s.TemporalEnabled = true
	if w := request("GET", path, "", false); w.Code != 401 {
		t.Fatal("private fact exposed", w.Code)
	}
	body := `{"claim_id":"` + c.ID + `","expected_head_revision":1,"validity":"unknown","freshness_class":"stable_preference"}`
	if w := request("POST", "/api/v4/memory/temporal", body, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/v4/memory/temporal", body, true); w.Code != 409 {
		t.Fatal("metadata CAS bypass", w.Code)
	}
	for _, date := range []string{"2026-02-01", "2026-02-01T10:00:00", "tomorrow"} {
		if w := request("GET", path+"&valid_at="+date, "", true); w.Code != 400 {
			t.Fatal("ambiguous instant admitted", date, w.Code)
		}
	}
	if w := request("GET", path+"&valid_at=2026-02-01T00:00:00Z", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"unknown_validity":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
