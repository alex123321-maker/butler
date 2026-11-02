package config

import "testing"

func TestExternalAuthRequiresOperatorComposition(t *testing.T) {
	base := OrchestratorConfig{ExternalAuthEnabled: true, ExternalAuthRPURL: "https://rp.example/", ExternalAuthClientFile: "/private/google.json", BrowserControlURL: "http://broker/internal/server-browser", ActionAuthorityTokenFile: "/private/broker", OwnerControlOwnerID: "owner", OwnerControlTokenFile: "/private/owner"}
	if validateExternalAuthConfig(base) != nil {
		t.Fatal("valid operator composition rejected")
	}
	for _, mutate := range []func(*OrchestratorConfig){func(c *OrchestratorConfig) { c.ExternalAuthClientFile = "" }, func(c *OrchestratorConfig) { c.OwnerControlTokenFile = "" }, func(c *OrchestratorConfig) { c.OwnerControlOwnerID = "" }, func(c *OrchestratorConfig) { c.ActionAuthorityTokenFile = "" }, func(c *OrchestratorConfig) { c.BrowserControlURL = "" }, func(c *OrchestratorConfig) { c.ExternalAuthRPURL = "http://rp.example/" }, func(c *OrchestratorConfig) { c.ExternalAuthRPURL = "https://rp.example/?code=private" }, func(c *OrchestratorConfig) { c.ExternalAuthRPURL = "https://other:private@rp.example/" }} {
		cfg := base
		mutate(&cfg)
		if validateExternalAuthConfig(cfg) == nil {
			t.Fatal("incomplete operator composition enabled")
		}
	}
	if validateExternalAuthConfig(OrchestratorConfig{}) != nil {
		t.Fatal("default-off feature requires credentials")
	}
}
