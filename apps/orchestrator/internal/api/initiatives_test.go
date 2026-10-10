package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInitiativeAPIRequiresSeparateOwnerProof(t *testing.T) {
	token := strings.Repeat("synthetic-owner-token", 3)
	s := NewInitiativeServer(nil, NewOwnerControlAuth(token, "fixture-owner"))
	for _, authorization := range []string{"", "Bearer extension-token", "Bearer " + token} {
		r := httptest.NewRequest("POST", "/api/v3/initiatives/observations", strings.NewReader(`{"owner_id":"fixture-owner"}`))
		r.Header.Set("Authorization", authorization)
		w := httptest.NewRecorder()
		s.Handle().ServeHTTP(w, r)
		want := 401
		if authorization == "Bearer "+token {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("body or extension proof admitted owner operation: %d want %d", w.Code, want)
		}
	}
}
