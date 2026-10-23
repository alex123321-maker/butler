package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

type ownerSearchFixture struct {
	calls   int
	items   []catalog.BrowseItem
	failure error
	actor   owneridentity.Actor
}

func (f *ownerSearchFixture) Candidates(ctx context.Context, _ string) ([]catalog.BrowseItem, []string, error) {
	f.calls++
	f.actor, _ = owneridentity.FromContext(ctx)
	return f.items, []string{"semantic_indexed_only"}, f.failure
}

func TestOwnerSearchHTTPHybridAndFallbackIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	store := catalog.New(pool, manifest)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "setup"})
	first, err := store.Put(ctx, catalog.PutSource{ExternalKey: "one", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Земля в горшке должна просохнуть."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(ctx, catalog.PutSource{ExternalKey: "two", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Поливать растение вечером."})
	if err != nil {
		t.Fatal(err)
	}
	fake := &ownerSearchFixture{items: []catalog.BrowseItem{{Source: first}}}
	token := strings.Repeat("s", 40)
	server := &MemoryCatalogServer{Store: store, Auth: NewOwnerControlAuth(token, "owner"), Search: fake}
	call := func(body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v4/memory/search", strings.NewReader(body))
		req.Header.Set("X-Butler-Delivery-ID", "search")
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		server.Handle().ServeHTTP(w, req)
		return w
	}
	raw := `{"query":"растение","limit":6,"strategy":"hybrid"}`
	if w := call(raw, false); w.Code != 401 || fake.calls != 0 {
		t.Fatal(w.Code, fake.calls)
	}
	w := call(raw, true)
	var page catalog.BrowsePage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Mode != "hybrid" || len(page.Items) != 2 || page.NextOffset != 1 || fake.actor.OwnerID != "owner" {
		t.Fatal(w.Code, w.Body.String(), fake.actor)
	}
	found := false
	for _, item := range page.Items {
		if item.ID == second.ID && item.Match == "text" {
			found = true
		}
	}
	if !found {
		t.Fatal(page)
	}
	fake.failure = errors.New("private provider failure must not be serialized")
	w = call(raw, true)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Mode != "text" || len(page.Items) != 1 || page.Items[0].ID != second.ID || strings.Contains(w.Body.String(), "private provider") {
		t.Fatal(w.Code, w.Body.String())
	}
	before := fake.calls
	w = call(`{"query":"растение","limit":6,"offset":1}`, true)
	if w.Code != 200 || fake.calls != before {
		t.Fatal("pagination repeated inference", w.Code, fake.calls)
	}
}
