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
	"strings"
	"sync/atomic"
	"testing"
)

func loginTestArgs() browserauth.Args {
	return browserauth.Args{Scope: browserauth.Scope{Browser: "browser", ExpectedURL: egresstest.Origin + "/login", TargetURL: egresstest.Origin + "/session", Form: "form", Username: "#u", Password: "#p"}, Version: 1, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: "creq_test", Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: "creq_test", Field: "password"}}
}
func TestLoginPrivateArguments(t *testing.T) {
	a := loginTestArgs()
	raw, _ := json.Marshal(a)
	creds := []*runtimev1.ResolvedCredential{{Alias: "creq_test", Field: "username", Value: "private-user"}, {Alias: "creq_test", Field: "password", Value: "private-password"}}
	request, err := decodeRequest(browserauth.Tool, string(raw), creds)
	if err != nil || request.Login == nil || request.Login.Password != "private-password" || request.LoginPhase != "" {
		t.Fatal("private resolution", err)
	}
	for _, bad := range [][]*runtimev1.ResolvedCredential{nil, creds[:1], {creds[0], creds[0]}, {creds[0], {Alias: "other", Field: "password", Value: "private-password"}}} {
		if _, err := decodeRequest(browserauth.Tool, string(raw), bad); err == nil {
			t.Fatal("unresolved login accepted")
		}
	}
	out := buildResultJSON(browserauth.Tool, Result{OK: true, LoginNonce: "private-nonce", LoginURL: "private-url", LoginCookies: []Cookie{{Name: "auth", Value: "private-cookie"}}, LoginPosts: 1})
	if strings.Contains(out, "private") || strings.Contains(out, "login_") {
		t.Fatal("private protocol leaked")
	}
}
func TestLoginIndependentAccessVerification(t *testing.T) {
	for _, mode := range []string{"challenge", "redirect", "public", "auth_redirect", "foreign_redirect", "wrong_cookie", "rotating_cookie", "large_body", "foreign_cookie", "query", "private_target"} {
		t.Run(mode, func(t *testing.T) {
			var reads atomic.Int32
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads.Add(1)
				if r.Method != "GET" {
					t.Error("verification issued a write")
				}
				cookie, err := r.Cookie("auth")
				auth := err == nil && cookie.Value == "private-cookie"
				if mode == "public" {
					fmt.Fprint(w, "public success")
					return
				}
				if auth {
					if mode == "auth_redirect" {
						http.Redirect(w, r, "/login", 302)
						return
					}
					if mode == "rotating_cookie" {
						http.SetCookie(w, &http.Cookie{Name: "auth", Value: "rotated"})
					}
					if mode == "large_body" {
						fmt.Fprint(w, strings.Repeat("x", 65537))
						return
					}
					fmt.Fprint(w, "private account")
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, "/login", 302)
					return
				}
				if mode == "foreign_redirect" {
					http.Redirect(w, r, "https://foreign.example/login", 302)
					return
				}
				w.WriteHeader(401)
			}))
			defer site.Close()
			scope := loginTestArgs().Scope
			cookies := []Cookie{{Name: "auth", Value: "private-cookie", Domain: "external.test", Path: "/"}}
			landing := egresstest.Origin + "/account"
			if mode == "wrong_cookie" {
				cookies[0].Value = "wrong"
			}
			if mode == "foreign_cookie" {
				cookies[0].Domain = "other.test"
			}
			if mode == "query" {
				landing += "?token=x"
			}
			if mode == "private_target" {
				landing = "http://127.0.0.1/account"
			}
			accepted := verifyLoginAccess(context.Background(), egresstest.Policy(site), scope, landing, cookies)
			if accepted != (mode == "challenge" || mode == "redirect") {
				t.Fatal("access differential incorrectly classified")
			}
			if reads.Load() > 2 {
				t.Fatal("probe followed a redirect")
			}
		})
	}
}
