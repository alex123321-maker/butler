package browsersso

import (
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browserpage"
	"strings"
	"testing"
	"time"
)

func TestSSOExecutionStrictReferencesAndNetworkCaps(t *testing.T) {
	s := Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.test/", Trigger: "#login", IdPURL: "https://idp.test/authorize", TargetURL: "https://idp.test/login", VerifyURL: "https://rp.test/account", Form: "form", Username: "#u", Password: "#p"}
	hash, _ := s.Digest(0, 1)
	alias := "ssoq_" + strings.Repeat("d", 64)
	a := Args{Scope: s, Digest: hash, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "password"}}
	raw, _ := json.Marshal(a)
	if _, e := Parse(string(raw)); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"scope_digest":`, `"scope_digest":"`+hash+`","scope_digest":`, 1), strings.Replace(string(raw), alias, "creq_"+strings.Repeat("d", 64), 1), strings.TrimSuffix(string(raw), "}") + `,"raw_password":"secret"}`} {
		if _, e := Parse(bad); e == nil {
			t.Fatal("ambiguous or unbound input accepted")
		}
	}
	d := Dispatch{Page: browserpage.Binding{Ref: s.RPPage, URL: s.RPURL, Epoch: 0, RuntimeVersion: 1}, Digest: hash, ExpiresAt: time.Now().Add(time.Minute), Allowed: []NetworkScope{{Origin: "https://rp.test", PathPrefix: "/"}, {Origin: "https://idp.test", PathPrefix: "/"}}}
	if !d.Valid(s) {
		t.Fatal("complete private dispatch rejected")
	}
	for _, bad := range []NetworkScope{{Origin: "https://rp.test/path", PathPrefix: "/"}, {Origin: "https://rp.test", PathPrefix: "/a/../"}, {Origin: "https://rp.test", PathPrefix: "/?query=bad"}} {
		invalid := d
		invalid.Allowed = append(append([]NetworkScope{}, d.Allowed...), bad)
		if invalid.Valid(s) {
			t.Fatal("noncanonical private network scope accepted")
		}
	}
	d.Denied = []NetworkScope{{Origin: "https://idp.test", PathPrefix: "/login"}}
	if d.Valid(s) || d.Allows(s.TargetURL) || d.Allows("https://idp.test/login/sub?code=private") {
		t.Fatal("path denial bypassed")
	}
	if !d.Allows("https://idp.test/loginfoo") {
		t.Fatal("path prefix lacks segment boundary")
	}
	for _, path := range []string{"/%2e%2e/login", "/a/../login", "/a%2flogin", "/a%5clogin"} {
		if d.Allows("https://idp.test" + path) {
			t.Fatal("noncanonical path accepted")
		}
	}
	d.Denied = nil
	d.Allowed = d.Allowed[:1]
	if d.Valid(s) {
		t.Fatal("RP-only dispatch admitted")
	}
}
