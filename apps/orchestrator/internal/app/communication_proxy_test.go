package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCommunicationNativeProxyAcceptsPooledHTTP2(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("synthetic response"))
	}))
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	defer upstream.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "example.com:443" {
			t.Error("unexpected synthetic proxy destination")
			w.WriteHeader(400)
			return
		}
		remote, err := net.Dial("tcp", upstream.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			w.WriteHeader(502)
			return
		}
		defer remote.Close()
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buffered.Flush()
		go io.Copy(remote, buffered)
		io.Copy(conn, remote)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	transport := upstream.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	transport.ForceAttemptHTTP2 = true
	// The test server's generated certificate is for example.com, trusted only
	// by this test client. Production TLS settings are never changed.
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	route := &communicationModelRoute{t: t, proxyAddresses: map[string]bool{proxy.Listener.Addr().String(): true}}
	for n := 0; n < 2; n++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(route.context(ctx), http.MethodPost, "https://example.com/", nil)
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			t.Fatal("proxied HTTP/2 continuation cancelled", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		cancel()
		if resp.StatusCode != 200 || resp.ProtoMajor != 2 {
			t.Fatal("expected synthetic HTTP/2 response")
		}
	}
	if route.requested.Load() != 1 || route.connected.Load() != 2 || route.denied.Load() != 0 {
		t.Fatal("proxy pool was bypassed or misclassified", route.requested.Load(), route.connected.Load(), route.denied.Load())
	}
}
