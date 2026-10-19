package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browsercontrol"
)

type browserOwnerFixture struct {
	calls   int
	command browsercontrol.Command
}

func (f *browserOwnerFixture) Control(_ context.Context, c browsercontrol.Command) (browsercontrol.Reply, error) {
	f.calls++
	f.command = c
	return browsercontrol.Reply{OK: true}, nil
}
func TestServerBrowserAuthenticatesHumanAndNeverAcceptsOwnerClaims(t *testing.T) {
	token := strings.Repeat("owner-proof-", 3)
	fixture := &browserOwnerFixture{}
	h := ServerBrowser(NewOwnerControlAuth(token, "actual-owner"), fixture)
	for _, tc := range []struct {
		body, token string
		status      int
	}{
		{`{"operation":"list"}`, "", 401},
		{`{"operation":"list","owner_id":"foreign"}`, token, 400},
		{`{"operation":"list","run_id":"foreign"}`, token, 400},
		{`{"operation":"list","operation":"take"}`, token, 400},
		{`{"operation":"text","text":"` + strings.Repeat("x", 17000) + `"}`, token, 400},
		{`{"operation":"frame","browser_session_id":"0123456789abcdef0123456789abcdef"}`, token, 200},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v4/server-browser", strings.NewReader(tc.body))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("owner data cacheable")
		}
	}
	if fixture.calls != 1 || fixture.command.OwnerID != "actual-owner" {
		t.Fatal("untrusted body supplied owner or reached private channel")
	}
}
