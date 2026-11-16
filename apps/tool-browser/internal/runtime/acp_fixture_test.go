package runtime

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browsercontrol"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/grpc"
)

// An opt-in, finite local HTTPS recipient plus real browser runtime. This seam
// exists only in a test executable; normal Butler keeps public-egress policy.
// Private account bytes are written only into its isolated owner-control volume.
func TestACPBrowserFixtureServer(t *testing.T) {
	if os.Getenv("BUTLER_ACP_BROWSER_FIXTURE") != "1" {
		t.Skip("isolated ACP browser fixture opt-in required")
	}
	dir := os.Getenv("BUTLER_ACP_BROWSER_EVIDENCE")
	if !filepath.IsAbs(dir) {
		t.Fatal("private isolated evidence path required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	claim, err := os.OpenFile(filepath.Join(dir, "fixture.once"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("fixture window already consumed")
	}
	claim.Close()
	cert, key := ssoTLS(t, t.TempDir())
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		t.Fatal("isolated TLS identity unavailable")
	}
	newValue := func() string {
		var b [24]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal("fixture entropy unavailable")
		}
		return hex.EncodeToString(b[:])
	}
	account := struct{ Username, Password, PIN, Fact string }{newValue(), newValue(), "583196", "ACP_ACCOUNT_" + newValue()[:12]}
	private, _ := json.Marshal(account)
	if err := os.WriteFile(filepath.Join(dir, "account.private.json"), private, 0600); err != nil {
		t.Fatal(err)
	}
	csrf, cookie := newValue(), newValue()
	var mu sync.Mutex
	counts := map[string]int{}
	journal := func(kind string) {
		mu.Lock()
		defer mu.Unlock()
		counts[kind]++
		data, _ := json.Marshal(counts)
		if err := os.WriteFile(filepath.Join(dir, "recipient.json"), data, 0600); err != nil {
			t.Error("recipient receipt unavailable")
		}
	}
	site := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/login":
			fmt.Fprintf(w, `<html><body><h1>Synthetic account</h1><form method="post" action="/session"><input name="user" id="u" autocomplete="username"><input name="password" id="p" type="password"><input type="hidden" name="csrf" value="%s"><button>Sign in</button></form></body></html>`, csrf)
		case "/session":
			journal("login_posts")
			if r.Method != "POST" || r.ParseForm() != nil || len(r.PostForm) != 3 || r.PostForm.Get("user") != account.Username || r.PostForm.Get("password") != account.Password || r.PostForm.Get("csrf") != csrf {
				journal("rejected_posts")
				http.Redirect(w, r, "/login", 303)
				return
			}
			journal("accepted_posts")
			http.SetCookie(w, &http.Cookie{Name: "session", Value: cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/account", 303)
		case "/account":
			c, err := r.Cookie("session")
			if err != nil || c.Value != cookie {
				journal("anonymous_denials")
				http.Redirect(w, r, "/login", 302)
				return
			}
			journal("authenticated_reads")
			fmt.Fprintf(w, "<html><body>Проверочный статус счёта: %s</body></html>", account.Fact)
		default:
			http.NotFound(w, r)
		}
	}))
	site.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	site.StartTLS()
	defer site.Close()
	roots := x509.NewCertPool()
	ca, err := os.ReadFile(os.Getenv("SSL_CERT_FILE"))
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("test TLS root unavailable")
	}
	p := egress.Policy{Resolver: ssoResolver{}, TLSRoots: roots, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, site.Listener.Addr().String())
	}}
	runner := NewPlaywrightRunnerWithPolicy("node", os.Getenv("BUTLER_BROWSER_E2E_SCRIPT"), p)
	server := NewServer(runner, nil)
	defer server.Close()
	server.RequireDispatchEpoch = true
	token, err := actionauth.ReadServiceToken(os.Getenv("BUTLER_BROWSER_CONTROL_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private service token required")
	}
	listener, err := net.Listen("tcp", ":9090")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(gs, server)
	go gs.Serve(listener)
	defer gs.Stop()
	mux := http.NewServeMux()
	mux.Handle(browsercontrol.Path, browsercontrol.Handler(token, server))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	httpServer := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go httpServer.ListenAndServe()
	defer httpServer.Close()
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("https://rp.sso.test/login\n"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(60 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("finite fixture lifetime ended")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
				return
			}
		}
	}
}
