package browserauth

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoginScopeClosedAndValueFree(t *testing.T) {
	a := Args{Scope: Scope{Browser: "b", ExpectedURL: "https://site.example/login", TargetURL: "https://site.example/session", Form: "form", Username: "#u", Password: "#p"}, Version: 1, UsernameRef: Ref{"credential_ref", "creq_x", "username"}, PasswordRef: Ref{"credential_ref", "creq_x", "password"}}
	raw, _ := json.Marshal(a)
	if _, err := Parse(string(raw)); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"origin", "userinfo", "query", "fragment", "selector", "version", "literal", "alias", "field", "extra", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			changed := a
			switch mode {
			case "origin":
				changed.TargetURL = "https://other.example/session"
			case "userinfo":
				changed.TargetURL = "https://user:secret@site.example/session"
			case "query":
				changed.ExpectedURL += "?secret=x"
			case "fragment":
				changed.ExpectedURL += "#x"
			case "selector":
				changed.Password = changed.Username
			case "version":
				changed.Version = 0
			case "literal":
				changed.PasswordRef.Type = "literal"
			case "alias":
				changed.PasswordRef.Alias = "other"
			case "field":
				changed.UsernameRef.Field = "password"
			}
			payload, _ := json.Marshal(changed)
			if mode == "extra" {
				payload = []byte(strings.TrimSuffix(string(payload), "}") + `,"login_phase":"submit"}`)
			}
			if mode == "trailing" {
				payload = append(payload, []byte(` {}`)...)
			}
			if _, err := Parse(string(payload)); err == nil {
				t.Fatal("unscoped login accepted")
			}
		})
	}
}
