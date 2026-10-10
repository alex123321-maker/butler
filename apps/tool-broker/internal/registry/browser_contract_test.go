package registry

import (
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"path/filepath"
	"testing"
)

func TestBrowserContractsRequireExplicitSessionAndSafeAuth(t *testing.T) {
	reg, err := Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "local")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"browser.read", "browser.open"} {
		valid, _, err := reg.Validate(&toolbrokerv1.ToolCall{ToolName: name, ArgsJson: `{"url":"https://example.com"}`})
		if !valid || err != nil {
			t.Fatalf("independent/open contract %s: %v", name, err)
		}
	}
	for _, name := range []string{"browser.navigate", "browser.snapshot", "browser.click", "browser.fill", "browser.type", "browser.wait_for", "browser.extract_text", "browser.set_cookie", "browser.restore_storage_state"} {
		valid, _, _ := reg.Validate(&toolbrokerv1.ToolCall{ToolName: name, ArgsJson: `{"url":"https://example.com","selector":"#a","value":"value","text":"text"}`})
		if valid {
			t.Fatalf("%s accepted implicit continuation", name)
		}
	}
	for _, args := range []string{
		`{"browser_session_id":"id","selector":"#a","value":"first"}`,
		`{"browser_session_id":"id","selector":"#a","value":{"type":"credential_ref","alias":"login","field":"password"},"expected_url":"https://example.com","observation_version":2}`,
	} {
		valid, _, _ := reg.Validate(&toolbrokerv1.ToolCall{ToolName: "browser.fill", ArgsJson: args})
		if valid != (args != `{"browser_session_id":"id","selector":"#a","value":"first"}`) {
			t.Fatalf("unexpected fill validity: %s", args)
		}
	}
	valid, _, _ := reg.Validate(&toolbrokerv1.ToolCall{ToolName: "browser.set_cookie", ArgsJson: `{"browser_session_id":"id","expected_url":"https://example.com","observation_version":1,"cookie":{"name":"sid","value":"raw","domain":"example.com"}}`})
	if valid {
		t.Fatal("cookie contract accepted raw auth material")
	}
	for _, name := range []string{"browser.set_cookie", "browser.restore_storage_state"} {
		contract, ok := reg.Get(name)
		if !ok || !contract.GetRequiresApproval() || !contract.GetSupportsCredentialRefs() {
			t.Fatalf("auth contract lacks broker mediation: %s", name)
		}
	}
	bind, _ := reg.Get("single_tab.bind")
	if bind.GetRuntimeTarget() != "tool-browser-local:9090" || bind.GetSupportsCredentialRefs() || bind.GetRequiresApproval() {
		t.Fatal("single-tab approval boundary changed")
	}
}
