package spaproxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/security/egress"
	"github.com/gorilla/websocket"
)

type fixtureResolver struct{}

func (fixtureResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func fixture(t *testing.T, h http.Handler) (*Proxy, *http.Client, *atomic.Bool) {
	t.Helper()
	cert, _, err := localCertificate("external.test", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(h)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	root, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	alive := &atomic.Bool{}
	alive.Store(true)
	policy := egress.Policy{Resolver: fixtureResolver{}, TLSRoots: roots, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:443" {
			return nil, egress.ErrDenied
		}
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	p, err := New(context.Background(), Options{URL: "https://external.test/", Deadline: time.Now().Add(10 * time.Second), Policy: policy, Allows: func(raw string) bool { return strings.HasPrefix(raw, "https://external.test/") }, Check: func(context.Context, string) error {
		if !alive.Load() {
			return browserspa.ErrDenied
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	proxy, _ := url.Parse(p.URL())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true, TLSClientConfig: pinnedTLS(p.SPKI)}, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(client.CloseIdleConnections)
	return p, client, alive
}

func pinnedTLS(pin string) *tls.Config {
	// Test client reproduces Chromium's exact ephemeral public-key trust, not
	// the relay's remote TLS verification (which stays enabled with test roots).
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(s tls.ConnectionState) error {
		if len(s.PeerCertificates) == 0 {
			return browserspa.ErrDenied
		}
		cert := s.PeerCertificates[0]
		digest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
		if base64.StdEncoding.EncodeToString(digest[:]) != pin || cert.VerifyHostname("external.test") != nil {
			return browserspa.ErrDenied
		}
		return nil
	}}
}

func TestRelayQueryAndWorkerScriptUseSameCheckedPath(t *testing.T) {
	var queries, scripts atomic.Int32
	p, client, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			queries.Add(1)
		} else {
			scripts.Add(1)
		}
		_, _ = io.WriteString(w, "checked")
	}))
	for _, path := range []string{"/sw.js", "/sw.js?update=2", "/cached-data"} {
		r, err := client.Get("https://external.test" + path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil || string(data) != "checked" {
			t.Fatal("GET positive control failed", err)
		}
	}
	r, err := client.Post("https://external.test/graphql", "application/json", strings.NewReader(`{"query":"query Q($filter:String!){catalog(filter:$filter){name}}","variables":{"filter":"books"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	r.Body.Close()
	stats, failure := p.Snapshot()
	if failure != "" || queries.Load() != 1 || scripts.Load() != 3 || stats.Queries != 1 {
		t.Fatal("query/script positive controls failed", failure)
	}
}

func TestRelayArbitraryPOSTIsDeniedBeforeRecipient(t *testing.T) {
	for _, body := range []string{`{"query":"mutation { buy }"}`, `{"value":"save"}`, `[{"query":"{a}"}]`} {
		t.Run(body, func(t *testing.T) {
			var effects atomic.Int32
			p, client, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1) }))
			r, _ := client.Post("https://external.test/graphql", "application/json", strings.NewReader(body))
			if r != nil {
				r.Body.Close()
			}
			_, failure := p.Snapshot()
			if failure != "query_denied" || effects.Load() != 0 {
				t.Fatal("unsupported POST reached recipient", failure)
			}
		})
	}
}

func TestRelayRevocationClosesIdleStreamAndCannotThaw(t *testing.T) {
	var effects atomic.Int32
	closed := make(chan struct{})
	p, client, alive := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	r, err := client.Get("https://external.test/events")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	data := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(r.Body, data); err != nil {
		t.Fatal(err)
	}
	alive.Store(false)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("revoked idle stream survived")
	}
	if _, err := client.Get("https://external.test/reconnect"); err == nil {
		t.Fatal("sealed relay reopened")
	}
	_, failure := p.Snapshot()
	if failure != "authority_denied" || effects.Load() != 1 {
		t.Fatal("revocation failed", failure)
	}
}

func TestRelayWSSReceivesDataButNeverForwardsBrowserData(t *testing.T) {
	var forwarded atomic.Int32
	closed := make(chan struct{})
	p, _, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		defer close(closed)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("update"))
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			forwarded.Add(1)
		}
	}))
	u, _ := url.Parse(p.URL())
	dial := websocket.Dialer{Proxy: http.ProxyURL(u), TLSClientConfig: pinnedTLS(p.SPKI), HandshakeTimeout: 3 * time.Second}
	conn, _, err := dial.Dial("wss://external.test/feed", http.Header{"Origin": []string{"https://external.test"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil || string(message) != "update" {
		t.Fatal("WSS positive control failed", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("buy")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("browser message did not seal socket")
	}
	_, failure := p.Snapshot()
	if failure != "client_message_denied" || forwarded.Load() != 0 {
		t.Fatal("browser message escaped", failure)
	}
}

func TestRelayLostQueryResponseDoesNotRetry(t *testing.T) {
	var effects atomic.Int32
	p, client, _ := fixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	r, err := client.Post("https://external.test/graphql", "application/json", strings.NewReader(`{"query":"{catalog{name}}"}`))
	if r != nil {
		r.Body.Close()
	}
	if err == nil {
		t.Fatal("lost response reported success")
	}
	r, _ = client.Post("https://external.test/graphql", "application/json", strings.NewReader(`{"query":"{catalog{name}}"}`))
	if r != nil {
		r.Body.Close()
	}
	_, failure := p.Snapshot()
	if effects.Load() != 1 || failure != "response_lost" {
		t.Fatal("lost query replayed", failure, effects.Load())
	}
}
