package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/credentials"
)

type externalRPStub struct{ starts, callbacks, reads int }

func (*externalRPStub) ExternalRPHost(host string) bool { return host == "rp.example" }
func (s *externalRPStub) StartExternal(_ context.Context, ticket string) (credentials.ExternalStart, error) {
	s.starts++
	if ticket != "private-ticket-canary" {
		return credentials.ExternalStart{}, credentials.ErrVault
	}
	return credentials.ExternalStart{URL: "https://accounts.google.com/o/oauth2/v2/auth?state=private-state-canary", CookieName: "__Host-butler-external-test", CookieValue: "private-cookie-canary", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (*externalRPStub) ExternalCallbackCookieName(_ context.Context, state string) (string, error) {
	if state != "private-state-canary" {
		return "", credentials.ErrVault
	}
	return "__Host-butler-external-test", nil
}
func (s *externalRPStub) CompleteExternalAuth(_ context.Context, state, code, cookie string, denied bool) error {
	s.callbacks++
	if state != "private-state-canary" || cookie != "private-cookie-canary" || !denied && code != "private-code-canary" {
		return credentials.ErrVault
	}
	return nil
}
func (s *externalRPStub) ExternalAccount(_ context.Context, name, cookie string) (credentials.ExternalRPAccount, error) {
	s.reads++
	if name != "__Host-butler-external-test" || cookie != "private-cookie-canary" {
		return credentials.ExternalRPAccount{}, credentials.ErrVault
	}
	return credentials.ExternalRPAccount{Authenticated: true, Reference: "rp-protected-reference"}, nil
}

func TestExternalRPHTTPPrivateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, host, cookie string
		status                           int
		starts, callbacks, reads         int
	}{
		{"root", "GET", "/", "rp.example", "", 200, 0, 0, 0},
		{"foreign-host", "GET", "/auth/google/start?ticket=private-ticket-canary", "foreign.example", "", 404, 0, 0, 0},
		{"owner-api", "GET", "/api/v4/credentials", "rp.example", "", 404, 0, 0, 0},
		{"post", "POST", "/auth/google/start?ticket=private-ticket-canary", "rp.example", "", 405, 0, 0, 0},
		{"duplicate-ticket", "GET", "/auth/google/start?ticket=private-ticket-canary&ticket=other", "rp.example", "", 400, 0, 0, 0},
		{"injected-return", "GET", "/auth/google/start?ticket=private-ticket-canary&return=https://evil.example", "rp.example", "", 400, 0, 0, 0},
		{"launch", "GET", "/auth/google/start?ticket=private-ticket-canary", "rp.example", "", 302, 1, 0, 0},
		{"missing-cookie", "GET", "/auth/google/callback?state=private-state-canary&code=private-code-canary", "rp.example", "", 409, 0, 0, 0},
		{"ambiguous-cookie", "GET", "/auth/google/callback?state=private-state-canary&code=private-code-canary", "rp.example", "__Host-butler-external-test=private-cookie-canary; __Host-butler-external-test=other", 409, 0, 0, 0},
		{"duplicate-code", "GET", "/auth/google/callback?state=private-state-canary&code=private-code-canary&code=other", "rp.example", "__Host-butler-external-test=private-cookie-canary", 400, 0, 0, 0},
		{"callback", "GET", "/auth/google/callback?state=private-state-canary&code=private-code-canary", "rp.example", "__Host-butler-external-test=private-cookie-canary", 200, 0, 1, 0},
		{"denied", "GET", "/auth/google/callback?state=private-state-canary&error=access_denied&error_description=private-error-canary", "rp.example", "__Host-butler-external-test=private-cookie-canary", 200, 0, 1, 0},
		{"anonymous", "GET", "/auth/google/account", "rp.example", "", 401, 0, 0, 0},
		{"account", "GET", "/auth/google/account", "rp.example", "__Host-butler-external-test=private-cookie-canary", 200, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &externalRPStub{}
			r := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
			r.Header.Set("Cookie", tc.cookie)
			w := httptest.NewRecorder()
			w.Header().Set("Access-Control-Allow-Origin", "*")
			BrowserExternalRP(s).ServeHTTP(w, r)
			if w.Code != tc.status || s.starts != tc.starts || s.callbacks != tc.callbacks || s.reads != tc.reads {
				t.Fatalf("boundary failed status=%d calls=%d/%d/%d", w.Code, s.starts, s.callbacks, s.reads)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Access-Control-Allow-Origin") != "" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
				t.Fatal("private response protections missing")
			}
			if tc.name != "launch" {
				for _, private := range []string{"private-state-canary", "private-code-canary", "private-cookie-canary", "private-error-canary", "private-ticket-canary"} {
					if strings.Contains(w.Body.String()+w.Header().Get("Location"), private) {
						t.Fatal("private callback material reflected")
					}
				}
			}
			if tc.name == "launch" {
				cookies := w.Result().Cookies()
				if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Domain != "" || cookies[0].Path != "/" {
					t.Fatal("browser binding cookie weakened")
				}
			}
		})
	}
}
