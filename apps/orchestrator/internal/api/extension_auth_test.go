package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtensionAuthMiddlewareDisabledWhenNoTokens(t *testing.T) {
	t.Parallel()

	middleware := NewExtensionAuthMiddleware(nil)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v2/extension/single-tab/session?session_key=x", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when extension API disabled, got %d", res.Code)
	}
}

func TestExtensionAuthMiddlewareRejectsMissingAuthorization(t *testing.T) {
	t.Parallel()

	middleware := NewExtensionAuthMiddleware([]string{"token-1"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v2/extension/single-tab/session?session_key=x", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when auth header is missing, got %d", res.Code)
	}
}

func TestExtensionAuthMiddlewareAcceptsBearerToken(t *testing.T) {
	t.Parallel()

	middleware := NewExtensionAuthMiddleware([]string{"token-1", "token-2"})
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v2/extension/single-tab/session?session_key=x", nil)
	req.Header.Set("Authorization", "Bearer token-2")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected wrapped handler status, got %d", res.Code)
	}
}

func TestParseBearerAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		ok     bool
		token  string
	}{
		{name: "valid", header: "Bearer abc", ok: true, token: "abc"},
		{name: "invalid scheme", header: "Basic abc", ok: false},
		{name: "invalid format", header: "Bearer", ok: false},
		{name: "empty", header: "", ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			token, ok := parseBearerAuthorization(tc.header)
			if ok != tc.ok {
				t.Fatalf("expected ok=%v, got %v", tc.ok, ok)
			}
			if token != tc.token {
				t.Fatalf("expected token %q, got %q", tc.token, token)
			}
		})
	}
}
