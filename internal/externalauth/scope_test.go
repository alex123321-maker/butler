package externalauth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browserpage"
)

func TestGoogleExternalScopeAndOperatorProfile(t *testing.T) {
	s := Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", Provider: "google", VerifyURL: "https://rp.example/auth/google/account"}
	cfg := GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-private-secret", RedirectURL: "https://rp.example/auth/google/callback"}
	p, e := ProfileFor(cfg, s.RPURL, s.VerifyURL)
	if e != nil || !p.Matches(s) {
		t.Fatal("valid fixed profile unavailable")
	}
	a := Request{RecordID: "cred_" + strings.Repeat("d", 64), Purpose: "One external sign-in", Scope: s}
	raw, _ := json.Marshal(a)
	if _, e := ParseRequest(string(raw)); e != nil {
		t.Fatal("valid request rejected")
	}
	for _, mode := range []string{"provider", "source_page", "callback_query", "verify_origin", "profile_callback_path", "profile_case", "duplicate", "private_url", "extra_field"} {
		t.Run(mode, func(t *testing.T) {
			x := a
			profile := p
			str := ""
			switch mode {
			case "provider":
				x.Scope.Provider = "other"
			case "source_page":
				x.Scope.RPPage.DocumentID = ""
			case "callback_query":
				x.Scope.RPURL += "?state=private-canary"
			case "verify_origin":
				x.Scope.VerifyURL = "https://other.example/auth/google/account"
			case "profile_callback_path":
				profile.RedirectURL = "https://rp.example/other/auth/google/callback"
			case "profile_case":
				profile.ClientFingerprint = strings.ToUpper(profile.ClientFingerprint)
			case "duplicate":
				str = strings.Replace(string(raw), `"purpose":`, `"purpose":"other",`+`"purpose":`, 1)
			case "private_url":
				str = strings.Replace(string(raw), `"external_scope":{`, `"external_scope":{"authorization_url":"https://accounts.google.com/auth?secret=private",`, 1)
			case "extra_field":
				str = strings.Replace(string(raw), `"record_id":`, `"owner_id":"foreign","record_id":`, 1)
			}
			if str != "" {
				if _, e := ParseRequest(str); e == nil {
					t.Fatal("ambiguous/private request accepted")
				}
				return
			}
			if profile.Matches(x.Scope) {
				t.Fatal("changed scope/profile accepted")
			}
		})
	}
	changed := cfg
	changed.ClientID = "456-test.apps.googleusercontent.com"
	p2, e := ProfileFor(changed, s.RPURL, s.VerifyURL)
	if e != nil || p2.ClientFingerprint == p.ClientFingerprint {
		t.Fatal("client replacement not pinned")
	}
	changed = cfg
	changed.ClientSecret = "rotated-synthetic-secret"
	p2, e = ProfileFor(changed, s.RPURL, s.VerifyURL)
	if e != nil || p2.ClientFingerprint != p.ClientFingerprint {
		t.Fatal("equivalent secret rotation changed client identity")
	}
}
