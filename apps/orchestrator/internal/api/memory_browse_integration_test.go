package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestMemoryOwnerBrowseNotesAndSearchIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("b", 40)
	server := &MemoryCatalogServer{Store: catalog.New(pool, manifest), Auth: NewOwnerControlAuth(token, "owner")}
	request := func(path, method, body, bearer, delivery string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if delivery != "" {
			r.Header.Set("X-Butler-Delivery-ID", delivery)
		}
		w := httptest.NewRecorder()
		server.Handle().ServeHTTP(w, r)
		return w
	}
	if w := request("/api/v4/memory/access", "GET", "", token, ""); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner access", w.Code)
	}
	for _, path := range []string{"/api/v4/memory/browse", "/api/v4/memory/search", "/api/v4/memory/notes"} {
		if w := request(path, "POST", "{", "", "x"); w.Code != 401 {
			t.Fatal("auth after parse", path, w.Code)
		}
	}
	note := `{"title":"Дома","body":"Полить растения вечером"}`
	w := request("/api/v4/memory/notes", "POST", note, token, "same-delivery")
	var first catalog.Source
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &first) != nil || first.OwnerID != "owner" || first.ScopeID != "owner" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("/api/v4/memory/notes", "POST", note, token, "same-delivery")
	var retry catalog.Source
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &retry) != nil || retry.ID != first.ID || retry.Revision != 1 {
		t.Fatal("lost ACK duplicated note", w.Code, w.Body.String())
	}
	for _, body := range []string{`{"owner_id":"other","body":"data"}`, `{"scope_id":"other","body":"data"}`, `{"title":"x","body":"data"} {}`} {
		if w = request("/api/v4/memory/notes", "POST", body, token, "forgery"); w.Code != 400 {
			t.Fatal("selected owner/trailing body", w.Code)
		}
	}
	if w = request("/api/v4/memory/search", "POST", `{"query":"растения"}`, token, ""); w.Code != 400 {
		t.Fatal("no delivery identity", w.Code)
	}
	w = request("/api/v4/memory/search", "POST", `{"query":"растение","limit":6,"offset":0}`, token, "search")
	var page catalog.BrowsePage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Mode != "text" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("search", w.Code, w.Body.String())
	}
	if w = request("/api/v4/memory/browse?limit=bad", "GET", "", token, ""); w.Code != 400 {
		t.Fatal("invalid pagination", w.Code)
	}
	w = request("/api/v4/memory/browse?limit=6", "GET", "", token, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Mode != "recent" || len(page.Items) != 1 {
		t.Fatal("recent", w.Code, w.Body.String())
	}
}

func TestMemoryOwnerAccessDoesNotDependOnCatalogReadiness(t *testing.T) {
	token := strings.Repeat("a", 40)
	server := &MemoryCatalogServer{Auth: NewOwnerControlAuth(token, "owner")}
	for _, auth := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/api/v4/memory/access", nil)
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		server.Handle().ServeHTTP(w, r)
		want := 401
		if auth {
			want = 200
		}
		if w.Code != want {
			t.Fatal("catalog outage changed authentication", w.Code)
		}
	}
}
