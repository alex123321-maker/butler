package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/browserauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserLoginChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires isolated Chromium")
	}
	for _, mode := range []string{"verified", "snapshot_selectors", "ambiguous_form", "public_landing", "formdata_mutation", "wrong_password", "foreign_target", "second_password", "extra_control", "business_hidden", "stale"} {
		t.Run(mode, func(t *testing.T) {
			began := time.Now()
			defer func() {
				t.Logf("operation=login/%s duration_ms=%.3f model_calls=0", mode, float64(time.Since(began).Microseconds())/1000)
			}()
			var posts, accounts, traps atomic.Int32
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				switch r.URL.Path {
				case "/session":
					posts.Add(1)
					_ = r.ParseForm()
					if r.Method != "POST" || r.PostForm.Get("user") != "private-login-user" || r.PostForm.Get("password") != "private-login-password" || r.PostForm.Get("csrf") != "private-csrf-canary" || len(r.PostForm) != 3 || mode == "wrong_password" {
						http.Redirect(w, r, "/login", 303)
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "auth", Value: "private-cookie-canary", Path: "/", HttpOnly: true})
					http.Redirect(w, r, "/account", 303)
				case "/account":
					cookie, err := r.Cookie("auth")
					if mode != "public_landing" && (err != nil || cookie.Value != "private-cookie-canary") {
						http.Redirect(w, r, "/login", 302)
						return
					}
					accounts.Add(1)
					fmt.Fprint(w, `<body>Account reference: B-432. private-cookie-canary private-login-password<script>fetch('/trap',{method:'POST'}).catch(()=>{});fetch('/trap').catch(()=>{})</script></body>`)
				case "/trap":
					traps.Add(1)
					w.WriteHeader(204)
				default:
					form := `<form method="post" action="/session"><input id="u" name="user"><input id="p" name="password" type="password"><input type="hidden" name="csrf" value="private-csrf-canary"><button>Login</button></form>`
					if mode == "snapshot_selectors" || mode == "ambiguous_form" {
						form = `<form method="post" action="/trap"><button>Other action</button></form>` + form
					}
					if mode == "foreign_target" {
						form = strings.ReplaceAll(form, `action="/session"`, `action="http://foreign.test/session"`)
					}
					if mode == "second_password" {
						form = strings.ReplaceAll(form, `<button>`, `<input name="other" type="password"><button>`)
					}
					if mode == "extra_control" {
						form = strings.ReplaceAll(form, `<button>`, `<input name="extra"><button>`)
					}
					if mode == "business_hidden" {
						form = strings.ReplaceAll(form, `<button>`, `<input type="hidden" name="amount" value="99"><button>`)
					}
					if mode == "formdata_mutation" {
						form += `<script>document.forms[0].addEventListener('formdata',e=>e.formData.set('csrf','mutated'))</script>`
					}
					fmt.Fprint(w, "<html><body>"+form+"</body></html>")
				}
			}))
			defer site.Close()
			script := os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")
			if script == "" {
				script = filepath.Join("..", "..", "scripts", "browser_runtime.mjs")
			}
			runner := NewPlaywrightRunnerWithPolicy("node", script, egresstest.Policy(site))
			runner.workspaceRoot, _ = filepath.Abs(filepath.Join("..", "..", "..", ".."))
			if filepath.IsAbs(script) {
				runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(script), "..", "..", ".."))
			}
			server := NewServer(runner, nil)
			defer server.Close()
			open := browserCall("browser.open", "run-1", map[string]any{"url": egresstest.Origin + "/login"})
			open.Contract.AllowedDomains = []string{"external.test"}
			response, err := server.Execute(context.Background(), open)
			if err != nil {
				t.Fatal(err)
			}
			var obs Result
			if json.Unmarshal([]byte(response.Result.ResultJson), &obs) != nil || !obs.OK {
				t.Fatal("login page unavailable", response.Result.GetError())
			}
			a := loginTestArgs()
			a.Browser = obs.SessionID
			a.Version = obs.ObservationVersion
			if mode == "snapshot_selectors" {
				a.Form, a.Username, a.Password = "form >> nth=1", "#u >> nth=0", "#p >> nth=0"
			}
			if mode == "stale" {
				a.Version++
			}
			payload, _ := json.Marshal(a)
			req := browserCall(browserauth.Tool, "run-1", nil)
			req.ToolCall.ArgsJson = string(payload)
			req.Contract = browserauth.Contract()
			req.Contract.AllowedDomains = []string{"external.test"}
			req.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Alias: "creq_test", Field: "username", Value: "private-login-user"}, {Alias: "creq_test", Field: "password", Value: "private-login-password"}}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			response, err = server.Execute(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(response.Result.ResultJson, "private-") || strings.Contains(response.Result.ResultJson, "login_cookies") || strings.Contains(response.Result.ResultJson, "login_nonce") {
				t.Fatal("private protocol leaked")
			}
			obs = Result{}
			if response.Result.ResultJson != "" {
				err = json.Unmarshal([]byte(response.Result.ResultJson), &obs)
			} else {
				err = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			verified := response.Result.Status == "completed" && obs.Authentication != nil && obs.Authentication.AuthenticatedRead && obs.Authentication.AnonymousChallenge
			if verified != (mode == "verified" || mode == "snapshot_selectors") {
				t.Fatalf("authentication classification: verified=%v status=%s error=%v", verified, response.Result.Status, response.Result.GetError())
			}
			if traps.Load() != 0 || posts.Load() > 1 {
				t.Fatal("login released extra traffic")
			}
			if (mode == "verified" || mode == "snapshot_selectors") && (posts.Load() != 1 || accounts.Load() != 2 || !strings.Contains(obs.Text, "B-432") || obs.NetworkState != "frozen") {
				t.Fatal("missing single POST, browser/probe reads or final observation", posts.Load(), accounts.Load())
			}
			if mode == "ambiguous_form" || mode == "foreign_target" || mode == "formdata_mutation" || mode == "second_password" || mode == "extra_control" || mode == "business_hidden" || mode == "stale" {
				if posts.Load() != 0 {
					t.Fatal("unsupported form dispatched")
				}
			}
		})
	}
}
