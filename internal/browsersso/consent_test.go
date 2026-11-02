package browsersso

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/browserpage"
)

func testScope() Scope {
	return Scope{Browser: strings.Repeat("c", 32), RPPage: browserpage.Ref{ID: strings.Repeat("a", 32), DocumentID: strings.Repeat("b", 32), Version: 1}, RPURL: "https://rp.example/", Trigger: "#sso", IdPURL: "https://idp.example/authorize", TargetURL: "https://idp.example/login", Form: "form", Username: "#u", Password: "#p", VerifyURL: "https://rp.example/account"}
}

func TestSSOConsentScope(t *testing.T) {
	s := testScope()
	if s.Validate() != nil {
		t.Fatal("valid conditional scope rejected")
	}
	for _, mutate := range []func(*Scope){
		func(s *Scope) { s.IdPURL = s.RPURL }, func(s *Scope) { s.TargetURL = "https://other.example/login" }, func(s *Scope) { s.VerifyURL = "https://other.example/account" }, func(s *Scope) { s.VerifyURL = s.RPURL }, func(s *Scope) { s.RPPage.Version = 0 }, func(s *Scope) { s.Browser = "active-tab" }, func(s *Scope) { s.Password = s.Username }, func(s *Scope) { s.Form = " " },
	} {
		bad := s
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("unbounded scope accepted")
		}
	}
	for _, raw := range []string{"http://idp.example/login", "https://user:pass@idp.example/login", "https://idp.example/login?state=private", "https://idp.example/login?", "https://idp.example/login#code", "https://IDP.example/login", "https://idp.example.:443/login", "https://idp.example:443/login", "https://idp.example:0444/login", "https://idp.example:70000/login", "https://idp.example/%2fother", "https://idp.example/a/../login", "https://idp.example//login", "https://idp.example", "https://idp.example/login\n", "https://idp.example/логин"} {
		if Origin(raw) != "" {
			t.Errorf("noncanonical/private URL accepted: %q", raw)
		}
	}
}

func TestSSOConsentDigestPinsGenerationAndAccount(t *testing.T) {
	s := testScope()
	hash, e := s.Digest(2, 3)
	if e != nil || len(hash) != 64 {
		t.Fatal("digest unavailable")
	}
	for _, mutate := range []func(*Scope){func(s *Scope) {
		s.IdPURL = "https://other.example/authorize"
		s.TargetURL = "https://other.example/login"
	}, func(s *Scope) { s.RPPage.Version++ }, func(s *Scope) { s.Trigger = "#other" }, func(s *Scope) { s.VerifyURL = "https://rp.example/other" }} {
		changed := s
		mutate(&changed)
		other, e := changed.Digest(2, 3)
		if e != nil || other == hash {
			t.Fatal("scope substitution not bound")
		}
	}
	for _, pair := range [][2]int64{{3, 3}, {2, 4}} {
		other, _ := s.Digest(pair[0], pair[1])
		if other == hash {
			t.Fatal("generation substitution not bound")
		}
	}
	if _, e := s.Digest(-1, 3); e == nil {
		t.Fatal("negative epoch accepted")
	}
}
