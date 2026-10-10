package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestMemoryOwnerAPIPrivateCorrectionAndDeleteIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("x", 40)
	s := &MemoryCatalogServer{Store: catalog.New(pool, manifest), Auth: NewOwnerControlAuth(token, "owner")}
	request := func(path, method, body, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		r.Header.Set("X-Butler-Delivery-ID", "fixture")
		w := httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v4/memory", "/api/v4/memory/export", "/api/v4/memory/citation?chunk_id=1"} {
		if w := request(path, "GET", "", ""); w.Code != http.StatusUnauthorized {
			t.Fatal(path, w.Code)
		}
	}
	w := request("/api/v4/memory/sources", "POST", `{"external_key":"private","source_kind":"owner_statement","scope_type":"user","scope_id":"owner","body":"Пиши по-русски"}`, token)
	var src catalog.Source
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &src) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	q, _ := json.Marshal(catalog.Correction{SourceID: src.ID, ExpectedRevision: 1, Body: "Пиши коротко по-русски", Title: "rule"})
	if w = request("/api/v4/memory/correct", "POST", string(q), token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("/api/v4/memory/correct", "POST", string(q), token); w.Code != 409 {
		t.Fatal("stale correction accepted", w.Code)
	}
	if w = request("/api/v4/memory/export", "GET", "", token); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Пиши по-русски")) {
		t.Fatal("export omitted original")
	}
	q, _ = json.Marshal(map[string]any{"source_id": src.ID, "expected_revision": 2, "action": "delete"})
	if w = request("/api/v4/memory/delete", "POST", string(q), token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("/api/v4/memory/export", "GET", "", token); w.Code != 200 || strings.Contains(w.Body.String(), "Пиши") {
		t.Fatal("private export text resurrected")
	}
	if w = request("/api/v4/memory/sources", "POST", `{"owner_id":"forged","source_kind":"owner_statement"}`, token); w.Code != 400 {
		t.Fatal("HTTP body selected owner")
	}
}
