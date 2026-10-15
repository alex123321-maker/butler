package readcontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictReadBoundsAndIdentity(t *testing.T) {
	for _, raw := range []string{`null`, `{"query":"x","owner_id":"owner"}`, `{"query":"x","query":"y"}`, `{"Query":"x"}`, `{"query":"x","source_revision":0}`, `{"query":"x","limit":11}`, `{"query":"x","page":10}`, `{"query":"x"} {}`} {
		if _, e := DecodeArgs(Search, raw); e == nil {
			t.Fatalf("ambiguous read admitted: %s", raw)
		}
	}
	valid := `{"source_id":"source_` + strings.Repeat("a", 64) + `","source_revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("b", 64) + `"}`
	if _, e := DecodeArgs(Open, valid); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(valid, `"source_revision":1`, `"source_revision":2.5`, 1), strings.Replace(valid, `"access_revision":1`, `"access_revision":null`, 1), strings.Replace(valid, `"source_id":"source_`, `"source_id":" source_`, 1), strings.TrimSuffix(valid, "}") + `,"max_chars":8193}`, strings.TrimSuffix(valid, "}") + `,"url":"https://example.invalid"}`} {
		if _, e := DecodeArgs(Open, bad); e == nil {
			t.Fatal("unbounded/forged source admitted")
		}
	}
	if _, e := DecodeArgs(Neighbors, valid); e == nil {
		t.Fatal("neighbors without catalog locator")
	}
}

type fixture struct{ calls int }

func (f *fixture) ReadDispatched(context.Context, Envelope) (Result, error) {
	f.calls++
	return Empty("empty"), nil
}
func TestPrivateReadHTTPDoesNotFollowRedirectOrProxy(t *testing.T) {
	f := &fixture{}
	token := strings.Repeat("synthetic-private-", 3)
	h, e := NewHandler(token, f)
	if e != nil {
		t.Fatal(e)
	}
	s := httptest.NewServer(h)
	defer s.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	c, e := NewClient(s.URL+Path, token)
	if e != nil {
		t.Fatal(e)
	}
	env := Envelope{RunID: "run", ToolCallID: "call", EffectID: "effect", Tool: Search, ArgsJSON: `{"query":"калибровка"}`, ContractHash: strings.Repeat("a", 64)}
	if r, e := c.ReadDispatched(context.Background(), env); e != nil || r.Status != "empty" || f.calls != 1 {
		t.Fatal(r, e)
	}
	raw, _ := json.Marshal(env)
	for _, suffix := range []string{"?x=1", "/other"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, Path+suffix, strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(w, req)
		if w.Code == 200 || f.calls != 1 {
			t.Fatal("alternate private read route")
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.URL+Path, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, _ = NewClient(redirect.URL+Path, token)
	if _, e = c.ReadDispatched(context.Background(), env); e == nil || f.calls != 1 {
		t.Fatal("redirect carried private read identity")
	}
}
