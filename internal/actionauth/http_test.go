package actionauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrivateAuthorityRequiresServiceToken(t *testing.T) {
	_, _, reader, _, _, _ := fixture(t)
	token := strings.Repeat("t", 48)
	server := httptest.NewServer(NewContextHandler(token, reader))
	defer server.Close()
	wrong, _ := NewHTTPContextReader(server.URL, strings.Repeat("x", 48))
	if _, err := wrong.GetActionContext(context.Background(), "run"); err == nil {
		t.Fatal("unauthenticated private action context disclosed")
	}
	client, err := NewHTTPContextReader(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetActionContext(context.Background(), "run")
	if err != nil || got.OwnerID != "owner" || got.LeaseEpoch != 4 {
		t.Fatalf("trusted context mismatch: %+v %v", got, err)
	}
}
func TestPrivateAuthorityDeniesFailureAndRedirect(t *testing.T) {
	for _, code := range []int{http.StatusServiceUnavailable, http.StatusForbidden, http.StatusFound} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("private service token forwarded across a redirect") }))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(code)
			}))
			defer server.Close()
			client, _ := NewHTTPContextReader(server.URL, strings.Repeat("t", 48))
			if _, err := client.GetActionContext(context.Background(), "run"); err == nil {
				t.Fatal("unavailable authority allowed")
			}
		})
	}
}

func TestPrivateAuthorityDoesNotUseAmbientProxy(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	configured := original.(*http.Transport).Clone()
	configured.Proxy = http.ProxyURL(proxyURL)
	http.DefaultTransport = configured
	t.Cleanup(func() { http.DefaultTransport = original; configured.CloseIdleConnections() })

	_, _, reader, _, _, _ := fixture(t)
	token := strings.Repeat("t", 48)
	server := httptest.NewServer(NewContextHandler(token, reader))
	defer server.Close()
	client, err := NewHTTPContextReader(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetActionContext(context.Background(), "run")
	if err != nil || got.OwnerID != "owner" || proxyCalls.Load() != 0 {
		t.Fatalf("private authority must bypass ambient proxy: %v", err)
	}
}
