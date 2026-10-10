package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPromptAuthorityCannotBeForgedByDocumentOrHeader(t *testing.T) {
	server := NewPromptServer(fakePromptManager{})
	server.SetOwnerControlAuth(NewOwnerControlAuth(strings.Repeat("p", 32), "owner"))
	for _, path := range []string{"/api/v1/prompts/system", "/api/v1/prompts/system/preview"} {
		method := http.MethodPut
		if strings.HasSuffix(path, "preview") {
			method = http.MethodPost
		}
		request := httptest.NewRequest(method, path, strings.NewReader(`{"base_prompt":"foreign instruction", "user_id":"owner"}`))
		request.Header.Set("X-Butler-Owner-ID", "owner")
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		response := httptest.NewRecorder()
		if method == http.MethodPut {
			server.HandleConfig().ServeHTTP(response, request)
		} else {
			server.HandlePreview().ServeHTTP(response, request)
		}
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s: got %d", path, response.Code)
		}
	}
}
