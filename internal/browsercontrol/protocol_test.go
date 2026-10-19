package browsercontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type controlFunc func(context.Context, Command) (Reply, error)

func (f controlFunc) Control(ctx context.Context, c Command) (Reply, error) { return f(ctx, c) }
func TestPrivateControlAuthenticationAndExactBoundedBodies(t *testing.T) {
	calls := 0
	token := strings.Repeat("t", 32)
	h := Handler(token, controlFunc(func(_ context.Context, c Command) (Reply, error) { calls++; return Reply{OK: true}, nil }))
	for _, tc := range []struct {
		body, auth string
		status     int
	}{
		{`{"operation":"frame"}`, "", 401},
		{`{"operation":"frame","operation":"take"}`, "Bearer " + token, 400},
		{`{"operation":"frame","runtime_target":"attacker"}`, "Bearer " + token, 400},
		{`{"operation":"text","text":"` + strings.Repeat("x", 17000) + `"}`, "Bearer " + token, 400},
		{`{"operation":"frame"}`, "Bearer " + token, 200},
	} {
		r := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
	}
	if calls != 1 {
		t.Fatal("invalid control reached executor")
	}
}
func TestPrivateClientNeverFollowsRedirectWithAuthority(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	client, err := NewClient(redirect.URL+Path, strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Control(context.Background(), Command{Operation: "frame"}); err == nil || calls != 0 {
		t.Fatal("private redirect forwarded authority")
	}
}
