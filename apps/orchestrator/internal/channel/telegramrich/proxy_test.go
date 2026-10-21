package telegramrich

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunicationProxyKeepsTokenInsideVerifiedTLS(t *testing.T) {
	token := "123456789:abcdefghijklmnopqrstuvwxyzABCDEFGH"
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	// A hostile proxy can tunnel to an impersonator, but cannot make the client
	// send its token before the official host's certificate has been verified.
	upstreamCalls := make(chan bool, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls <- true
		w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true}}`))
	}))
	upstream.Config.ErrorLog = log.New(io.Discard, "", 0)
	upstream.StartTLS()
	defer upstream.Close()
	requests := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.RequestURI + " " + r.Header.Get("Authorization")
		if r.Method != http.MethodConnect || r.Host != "api.telegram.org:443" {
			t.Error("unexpected proxy destination")
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
	proxyFile := filepath.Join(t.TempDir(), "proxy")
	if err := os.WriteFile(proxyFile, []byte(proxy.URL), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := NewClientFromTokenFileWithProxyFile(tokenFile, proxyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer c.http.CloseIdleConnections()
	if _, err := c.GetMe(context.Background()); err != ErrUnavailable {
		t.Fatal("untrusted TLS endpoint accepted")
	}
	if got := <-requests; got != "CONNECT api.telegram.org:443 " || strings.Contains(got, token) {
		t.Fatal("proxy request exposed an unexpected destination or authorization")
	}
	select {
	case <-upstreamCalls:
		t.Fatal("token-bearing request reached impersonator")
	default:
	}
	if strings.Contains(fmt.Sprintf("%v %#v", c, c), token) {
		t.Fatal("client formatting leaked token")
	}
	transport := c.http.Transport.(*http.Transport)
	req, _ := http.NewRequest("GET", "https://other.example/", nil)
	if _, err := transport.Proxy(req); err != ErrUnavailable {
		t.Fatal("proxy allowed arbitrary host")
	}
	// The default path must not silently inherit a credential-bearing env proxy.
	t.Setenv("HTTPS_PROXY", proxy.URL)
	direct, err := NewClientFromTokenFile(tokenFile)
	if err != nil || direct.http.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("implicit environment proxy enabled")
	}
}

func TestCommunicationProxyFileValidationIsRedacted(t *testing.T) {
	for _, value := range []string{"", "socks5://host:1080", "http://", "file:///private", "http://host/path", "http://user:secret@host?secret=yes", "http://host#secret", strings.Repeat("x", 4097)} {
		path := filepath.Join(t.TempDir(), "proxy")
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readProxyFile(path); err != ErrUnavailable {
			t.Fatal("invalid proxy accepted")
		}
	}
	for _, value := range []string{"http://127.0.0.1:1080", "https://user:secret@proxy.example:443/"} {
		path := filepath.Join(t.TempDir(), "proxy")
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readProxyFile(path); err != nil {
			t.Fatal("valid proxy rejected")
		}
	}
}
