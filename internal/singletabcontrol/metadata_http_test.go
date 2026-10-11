package singletabcontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type testMetadata struct{ calls atomic.Int32 }

func (m *testMetadata) List(context.Context) ([]Tab, error) {
	m.calls.Add(1)
	return []Tab{{BrowserInstance: "fixed-host", Ref: "7", Origin: "https://example.invalid", Active: true}}, nil
}
func (m *testMetadata) Check(context.Context, Tab) error { m.calls.Add(1); return nil }
func TestSingleTabResourcePrivateMetadataHTTP(t *testing.T) {
	backend := &testMetadata{}
	token := strings.Repeat("metadata-fixture-", 3)
	h, e := NewMetadataHandler(backend, token)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	// This client ignores even a hostile ambient default proxy.
	old := http.DefaultTransport
	http.DefaultTransport = &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return url.Parse("http://127.0.0.1:1") }}
	defer func() { http.DefaultTransport = old }()
	client, e := NewMetadataClient(server.URL, token)
	if e != nil {
		t.Fatal(e)
	}
	tabs, e := client.List(context.Background())
	if e != nil || len(tabs) != 1 || !tabs[0].Active {
		t.Fatal("fixed metadata denied", e)
	}
	if e = client.Check(context.Background(), tabs[0]); e != nil {
		t.Fatal(e)
	}
	before := backend.calls.Load()
	for _, q := range []struct {
		body, auth string
		status     int
	}{
		{`{"operation":"list"}`, "", 401},
		{`{"operation":"click"}`, "Bearer " + token, 403},
		{`{"operation":"list","url":"https://example.invalid"}`, "Bearer " + token, 400},
		{`{"operation":"check","tab":{"browser_instance_id":"fixed-host","tab_ref":"7","origin":"https://u:p@example.invalid"}}`, "Bearer " + token, 403},
		{`{"operation":"check","tab":{"browser_instance_id":"","tab_ref":"7","origin":"https://example.invalid"}}`, "Bearer " + token, 403},
	} {
		r := httptest.NewRequest("POST", server.URL, strings.NewReader(q.body))
		if q.auth != "" {
			r.Header.Set("Authorization", q.auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != q.status {
			t.Fatal("unsafe metadata shape accepted", w.Code, q.status)
		}
	}
	if backend.calls.Load() != before {
		t.Fatal("rejected metadata invoked native path")
	}
	for _, u := range []string{"http://192.0.2.1:9090", "http://remote.test:9090", "http://user:pass@127.0.0.1", "http://127.0.0.1?token=x", "http://127.0.0.1#x"} {
		if _, e = NewMetadataClient(u, token); e == nil {
			t.Fatal("plaintext remote/reflected endpoint accepted")
		}
	}
}
