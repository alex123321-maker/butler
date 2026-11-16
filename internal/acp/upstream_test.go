package acp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGoogleUpstreamRetainsCheckedIPAndBufferedTLSBytes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodConnect || r.Host != "8.8.8.8:443" || r.Header.Get("Proxy-Authorization") != "" {
			t.Error("unchecked proxy request")
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\nfixture")
	}))
	defer server.Close()
	dial, err := googleUpstreamDial(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{"127.0.0.1:443", "169.254.169.254:443", "198.18.0.1:443", "10.0.0.1:443", "aicode.googleapis.com:443", "8.8.8.8:80"} {
		if _, err := dial(context.Background(), "tcp", destination); err == nil {
			t.Fatal("unchecked destination reached upstream", destination)
		}
	}
	conn, err := dial(context.Background(), "tcp", "8.8.8.8:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	raw, err := io.ReadAll(conn)
	if err != nil || string(raw) != "fixture" || requests.Load() != 1 {
		t.Fatal("tunnel prefix lost or denial dispatched", string(raw), err, requests.Load())
	}
}

func TestGoogleUpstreamRefusesCredentialsAndAmbiguousRoutes(t *testing.T) {
	for _, raw := range []string{"https://proxy:443", "http://user:password@proxy:80", "http://proxy:80/path", "http://proxy:80?token=value", "http://proxy", "socks5://proxy:1080"} {
		if _, err := googleUpstreamDial(raw); err == nil {
			t.Fatal("unsafe route accepted")
		}
	}
}
