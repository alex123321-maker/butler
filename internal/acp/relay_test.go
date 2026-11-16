package acp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestGoogleEgressDeniesNonGoogleAndPrivateTargets(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ServeGoogleEgress(ctx, listener) }()
	for _, host := range []string{"127.0.0.1", "169.254.169.254", "192.168.0.5", "example.com", "butler-main-orchestrator-1"} {
		conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(time.Second))
		fmt.Fprintf(conn, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n\r\n", host, host)
		response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
		if err != nil || response.StatusCode != 403 {
			t.Fatal("runtime egress escaped fixed Google destination policy", host, err)
		}
		response.Body.Close()
		conn.Close()
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLiveGoogleEgressTLSPreflight(t *testing.T) {
	socket := os.Getenv("BUTLER_ACP_LIVE_EGRESS_SOCKET")
	if socket == "" {
		t.Skip("explicit public TLS preflight requires an operator egress socket")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go ServeLocalProxy(ctx, listener, socket)
	proxy, _ := url.Parse("http://" + listener.Addr().String())
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, endpoint := range []string{"https://oauth2.googleapis.com/token", "https://aicode.googleapis.com/", "https://daily-cloudcode-pa.googleapis.com/", "https://lh3.googleusercontent.com/"} {
		request, _ := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("public Google TLS path unavailable; no account code should be requested", endpoint, err)
		}
		response.Body.Close()
		if response.StatusCode >= 500 {
			t.Fatal("public Google service unavailable", response.StatusCode)
		}
		t.Log("Verified Google TLS path", endpoint, "HEAD status", response.StatusCode, "no credentials or inference used")
	}
}
