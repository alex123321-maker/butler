package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerMaintenanceCannotBeInvokedByModelResourceOrForgedIdentity(t *testing.T) {
	auth := NewOwnerControlAuth(strings.Repeat("k", 32), "owner")
	called := 0
	handler := OwnerMaintenance(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(204) }))
	for _, path := range []string{"/api/v1/settings/BUTLER_BASE_SYSTEM_PROMPT", "/api/v1/settings/restart", "/api/v1/providers/openai_codex/auth/start"} {
		request := httptest.NewRequest("POST", path, strings.NewReader(`{"owner_id":"owner"}`))
		request.Header.Set("X-Owner-ID", "owner")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 401 || called != 0 {
			t.Fatal("foreign instruction reached maintenance handler")
		}
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("k", 32))
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 204 {
			t.Fatal("authenticated owner mutation denied")
		}
		called = 0
	}
}
