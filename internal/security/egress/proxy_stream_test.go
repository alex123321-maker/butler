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

func streamProxyClient(t *testing.T, proxy *ScopedProxy) *http.Client {
	t.Helper()
	u, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}, Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestProxySSEFlushAndFreeze(t *testing.T) {
	var effects, dials atomic.Int32
	cancelled := make(chan struct{})
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(recipient.Close)
	u, _ := url.Parse(recipient.URL)
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, u.Host, &dials), "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	client := streamProxyClient(t, proxy)
	response, err := client.Get("http://external.test/events")
	if err != nil {
		t.Fatal("first SSE event was buffered until stream completion:", err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatal("missing live event", line, err)
	}
	proxy.Freeze()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("recipient stream survived freeze")
	}
	_, err = io.ReadAll(reader)
	if err == nil {
		t.Fatal("cancelled stream was reported as a complete response")
	}
	denied, err := client.Get("http://external.test/reconnect")
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden || effects.Load() != 1 || dials.Load() != 1 {
		t.Fatal("frozen reconnect reached recipient")
	}
}

func TestProxyHTTPResponseAllowanceIsCumulativeAndTerminal(t *testing.T) {
	var effects, dials atomic.Int32
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		_, _ = io.WriteString(w, "12345")
	}))
	defer recipient.Close()
	u, _ := url.Parse(recipient.URL)
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, u.Host, &dials), "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	// A smaller instance of the production shared counter keeps this fixture tiny.
	proxy.inbound = NewCounter(8)
	client := streamProxyClient(t, proxy)
	first, err := client.Get("http://external.test/one")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(first.Body)
	first.Body.Close()
	if err != nil || string(body) != "12345" {
		t.Fatal("positive control failed", err)
	}
	second, err := client.Get("http://external.test/two")
	if err == nil {
		_, err = io.ReadAll(second.Body)
		second.Body.Close()
	}
	if err == nil {
		t.Fatal("aggregate response overflow had a clean success")
	}
	if err := proxy.AllowUntil(time.Now().Add(time.Second)); err == nil {
		t.Fatal("thaw reset exhausted response allowance")
	}
	third, err := client.Get("http://external.test/three")
	if err != nil {
		t.Fatal(err)
	}
	third.Body.Close()
	if third.StatusCode != http.StatusForbidden || effects.Load() != 2 || dials.Load() != 2 {
		t.Fatal("exhausted context reached recipient again")
	}
}

func TestProxyHTTPAndCONNECTShareResponseAllowance(t *testing.T) {
	var dials atomic.Int32
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "http")
	}))
	defer recipient.Close()
	u, _ := url.Parse(recipient.URL)
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
	policy := proxyFixturePolicy(t, u.Host, &dials)
	httpDial := policy.Dial
	policy.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if strings.HasSuffix(address, ":443") {
			return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
		}
		return httpDial(ctx, network, address)
	}
	proxy, err := NewBrowserProxy(policy, "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxy.inbound = NewCounter(7)
	proxyURL, _ := url.Parse(proxy.URL())
	conn, err := net.DialTimeout("tcp", proxyURL.Host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = fmt.Fprint(conn, "CONNECT external.test:443 HTTP/1.1\r\nHost: external.test:443\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("CONNECT positive control failed", err)
	}
	var remote net.Conn
	select {
	case remote = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("missing tunnel recipient")
	}
	defer remote.Close()
	_ = remote.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = remote.Write([]byte("tls"))
	got := make([]byte, 3)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "tls" {
		t.Fatal("tunnel positive control failed", err)
	}
	plain, err := streamProxyClient(t, proxy).Get("http://external.test/plain")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(plain.Body)
	plain.Body.Close()
	if err != nil || string(body) != "http" {
		t.Fatal("mixed transport positive control failed", err)
	}
	_, _ = remote.Write([]byte("overflow"))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("tunnel used a separate response allowance")
	}
	if err := proxy.AllowUntil(time.Now().Add(time.Second)); err == nil {
		t.Fatal("tunnel byte exhaustion was reset")
	}
}

func TestProxyOutboundAllowanceExhaustionIsTerminal(t *testing.T) {
	var effects, dials atomic.Int32
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	defer recipient.Close()
	u, _ := url.Parse(recipient.URL)
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, u.Host, &dials), "http://external.test", time.Now().Add(time.Minute), 512)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	client := streamProxyClient(t, proxy)
	positive, err := client.Get("http://external.test/control")
	if err != nil {
		t.Fatal(err)
	}
	positive.Body.Close()
	if positive.StatusCode != http.StatusOK || effects.Load() != 1 {
		t.Fatal("outbound positive control failed")
	}
	denied, err := client.Post("http://external.test/effect", "text/plain", strings.NewReader(strings.Repeat("x", 512)))
	if err != nil {
		t.Fatal(err)
	}
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden || effects.Load() != 1 || dials.Load() != 1 {
		t.Fatal("over-budget payload reached recipient")
	}
	if err := proxy.AllowUntil(time.Now().Add(time.Second)); err == nil {
		t.Fatal("outbound exhaustion reopened")
	}
}

func TestProxyConcurrentAdmissionIsBoundedBeforeDNS(t *testing.T) {
	var dials atomic.Int32
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, "unused", &dials), "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for range maxProxyInflight {
		_, _, done, allowed := proxy.begin(context.Background())
		if !allowed {
			t.Fatal("capacity positive control failed")
		}
		releases = append(releases, done)
	}
	response, err := streamProxyClient(t, proxy).Get("http://external.test/overflow")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || dials.Load() != 0 {
		t.Fatal("capacity denial reached DNS/dial")
	}
	releases[0]()
	releases = releases[1:]
	_, _, done, allowed := proxy.begin(context.Background())
	defer done()
	if !allowed {
		t.Fatal("completed request did not release capacity")
	}
}

func TestProxyReconnectAllowanceSurvivesFreezeAndThaw(t *testing.T) {
	var dials atomic.Int32
	proxy, err := NewScopedProxy(proxyFixturePolicy(t, "unused", &dials), "http://external.test", time.Now().Add(time.Minute), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	for range maxProxyRequests {
		_, _, done, allowed := proxy.begin(context.Background())
		if !allowed {
			t.Fatal("attempt positive control failed")
		}
		done()
	}
	proxy.Freeze()
	if err := proxy.AllowUntil(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	response, err := streamProxyClient(t, proxy).Get("http://external.test/reconnect")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || dials.Load() != 0 {
		t.Fatal("attempt exhaustion was reset by thaw")
	}
	if err := proxy.AllowUntil(time.Now().Add(time.Second)); err == nil {
		t.Fatal("exhausted context reopened")
	}
}
