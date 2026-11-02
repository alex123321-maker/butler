package config

import (
	"fmt"
	"net/url"
	"strings"
)

func externalAuthOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_BROWSER_EXTERNAL_AUTH_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.ExternalAuthEnabled = v == "true" }},
		{key: "BUTLER_BROWSER_EXTERNAL_AUTH_CLIENT_FILE", component: "orchestrator", typeName: "string", defaultValue: "", isSecret: true, requiresRestart: true, assign: func(v string) { cfg.ExternalAuthClientFile = strings.TrimSpace(v) }},
		{key: "BUTLER_BROWSER_EXTERNAL_AUTH_RP_URL", component: "orchestrator", typeName: "string", defaultValue: "", requiresRestart: true, assign: func(v string) { cfg.ExternalAuthRPURL = strings.TrimSpace(v) }},
	}
}
func externalAuthBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_BROWSER_EXTERNAL_AUTH_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.ExternalAuthEnabled = v == "true" }}}
}
func validateExternalAuthConfig(cfg OrchestratorConfig) error {
	if !cfg.ExternalAuthEnabled {
		return nil
	}
	u, e := url.Parse(cfg.ExternalAuthRPURL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "/" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || u.String() != cfg.ExternalAuthRPURL || u.Host != strings.ToLower(u.Host) || cfg.ExternalAuthClientFile == "" || cfg.BrowserControlURL == "" || cfg.ActionAuthorityTokenFile == "" || cfg.OwnerControlOwnerID == "" || cfg.OwnerControlToken == "" && cfg.OwnerControlTokenFile == "" {
		return fmt.Errorf("external browser authentication requires a canonical HTTPS RP, private client file, owner authentication and Browser/Broker controls")
	}
	return nil
}
