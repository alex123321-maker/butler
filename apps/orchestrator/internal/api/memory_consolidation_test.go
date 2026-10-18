package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
)

func TestConsolidationOwnerAPIRequiresOwnerAndQualifiedConsumer(t *testing.T) {
	s := &MemoryCatalogServer{Store: catalog.New(nil, nil), Auth: NewOwnerControlAuth(strings.Repeat("x", 40), "owner"), Consolidation: &consolidation.Store{}}
	for _, route := range []string{"prepare", "activate", "deactivate", "retire"} {
		r := httptest.NewRequest("POST", "/api/v4/memory/consolidation/"+route, strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("unauthenticated owner route", route, w.Code)
		}
		r = httptest.NewRequest("POST", "/api/v4/memory/consolidation/"+route, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
		w = httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("missing delivery", route, w.Code)
		}
	}
	for _, route := range []string{"prepare", "activate"} {
		r := httptest.NewRequest("POST", "/api/v4/memory/consolidation/"+route, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
		r.Header.Set("X-Butler-Delivery-ID", "owner-request")
		w := httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatal("unqualified model path admitted", route, w.Code)
		}
	}
}
