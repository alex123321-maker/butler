package egress

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func proxyFixturePolicy(t *testing.T, target string, dials *atomic.Int32) Policy {
	t.Helper()
	return Policy{Resolver: resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}), Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasPrefix(address, "93.184.216.34:") {
			return nil, ErrDenied
		}
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}}
}

func TestScopedProxyDeniesCrossOriginUpgradeAndCumulativeRelease(t *testing.T) {
	var effects, dials atomic.Int32
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/private", 302)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer recipient.Close()
	target, _ := url.Parse(recipient.URL)
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, target.Host, &dials), "http://external.test", time.Now().Add(time.Minute), 700)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for _, raw := range []string{"http://127.0.0.1/private", "http://other.test/path"} {
		resp, err := client.Get(raw)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 403 {
			t.Fatal("cross origin accepted")
		}
		resp.Body.Close()
	}
	req, _ := http.NewRequest("GET", "http://external.test/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 403 {
		t.Fatal("WebSocket upgrade accepted")
	}
	resp.Body.Close()
	resp, err = client.Get("http://external.test/redirect")
	if err != nil || resp.StatusCode != 302 {
		t.Fatal("redirect followed")
	}
	resp.Body.Close()
	for i := 0; i < 4; i++ {
		req, _ = http.NewRequest("POST", "http://external.test/action", strings.NewReader(strings.Repeat("x", 150)))
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if effects.Load() > 3 || effects.Load() < 2 {
		t.Fatalf("cumulative bound did not stop effects: %d", effects.Load())
	}
	if dials.Load() != effects.Load() {
		t.Fatal("denied request reached recipient")
	}
}

func TestCONNECTPinsDNSRejectsOtherHostsAndClosesExpiredTunnel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	var dials atomic.Int32
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, listener.Addr().String(), &dials), "https://external.test", time.Now().Add(400*time.Millisecond), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	parsed, _ := url.Parse(proxy.URL())
	deny, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprint(deny, "CONNECT 127.0.0.1:443 HTTP/1.1\r\nHost: 127.0.0.1:443\r\n\r\n")
	denied, _ := http.ReadResponse(bufio.NewReader(deny), &http.Request{Method: "CONNECT"})
	if denied.StatusCode != 403 || dials.Load() != 0 {
		t.Fatal("private CONNECT reached recipient")
	}
	denied.Body.Close()
	deny.Close()
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprint(conn, "CONNECT external.test:443 HTTP/1.1\r\nHost: external.test:443\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil || response.StatusCode != 200 {
		t.Fatal("allowed CONNECT failed")
	}
	remote := <-accepted
	defer remote.Close()
	_, _ = conn.Write([]byte("hello"))
	buffer := make([]byte, 5)
	remote.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = io.ReadFull(remote, buffer); err != nil || string(buffer) != "hello" {
		t.Fatal("allowed tunnel failed")
	}
	remote.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err = remote.Read(buffer); err == nil {
		t.Fatal("expired tunnel remained open")
	}
	if dials.Load() != 1 {
		t.Fatal("unexpected recipient connections")
	}
}

func TestCONNECTRejectsMixedDNSAndByteOverflow(t *testing.T) {
	var dials atomic.Int32
	policy := proxyFixturePolicy(t, "unused", &dials)
	policy.Resolver = resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.1")}}, nil
	})
	proxy, err := NewScopedProxy(policy, "https://external.test", time.Now().Add(time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	request := httptest.NewRequest("CONNECT", "http://external.test:443", nil)
	request.Host = "external.test:443"
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, request)
	if recorder.Code != 403 || dials.Load() != 0 {
		t.Fatal("mixed DNS accepted")
	}
	writer := &boundedWriter{writer: io.Discard, counter: NewCounter(5)}
	if _, err = writer.Write([]byte("12345")); err != nil {
		t.Fatal(err)
	}
	if n, err := writer.Write([]byte("6")); n != 0 || err != ErrLimit {
		t.Fatal("tunnel bytes exceeded cap")
	}
}

func TestScopedProxyFreezeCancelsInflightAndPersistsUntilBoundedWindow(t *testing.T) {
	var dials, effects atomic.Int32
	started, stopped := make(chan struct{}), make(chan struct{})
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		if r.URL.Path == "/slow" {
			close(started)
			<-r.Context().Done()
			close(stopped)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer recipient.Close()
	target, _ := url.Parse(recipient.URL)
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, target.Host, &dials), "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	finished := make(chan struct{})
	go func() {
		resp, err := client.Get("http://external.test/slow")
		if err == nil {
			resp.Body.Close()
		}
		close(finished)
	}()
	<-started
	proxy.Freeze()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("inflight HTTP survived freeze")
	}
	<-finished
	deny := func() {
		t.Helper()
		resp, err := client.Get("http://external.test/delayed")
		if err != nil || resp.StatusCode != 403 {
			t.Fatal("frozen request permitted")
		}
		resp.Body.Close()
	}
	deny()
	if dials.Load() != 1 || effects.Load() != 1 {
		t.Fatal("frozen request resolved or dispatched")
	}
	if err := proxy.AllowUntil(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get("http://external.test/click-window")
	if err != nil || resp.StatusCode != 200 {
		t.Fatal("bounded window unavailable")
	}
	resp.Body.Close()
	time.Sleep(150 * time.Millisecond)
	deny()
	if dials.Load() != 2 {
		t.Fatal("expired window permitted release")
	}
}

func TestScopedProxyFreezeClosesEstablishedCONNECT(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	var dials atomic.Int32
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, listener.Addr().String(), &dials), "https://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	parsed, _ := url.Parse(proxy.URL())
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprint(conn, "CONNECT external.test:443 HTTP/1.1\r\nHost: external.test:443\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil || response.StatusCode != 200 {
		t.Fatal("CONNECT failed")
	}
	remote := <-accepted
	defer remote.Close()
	proxy.Freeze()
	remote.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := remote.Read(make([]byte, 1)); err == nil {
		t.Fatal("frozen tunnel survived")
	}
	// Late connections also fail before DNS, not merely after connection setup.
	req := httptest.NewRequest("CONNECT", "http://external.test:443", nil)
	req.Host = "external.test:443"
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)
	if w.Code != 403 || dials.Load() != 1 {
		t.Fatal("frozen CONNECT reached resolver/dial")
	}
}
