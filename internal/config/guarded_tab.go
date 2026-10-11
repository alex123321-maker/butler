package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// This controls owner decisions only. It does not advertise a tool catalog or
// enable an executor. Stored settings cannot choose a credential destination.
func guardedTabOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_GUARDED_TAB_OWNER_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.GuardedTabOwnerEnabled = v == "true" }},
		{key: "BUTLER_GUARDED_TAB_HOST", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(v string) { cfg.GuardedTabHost = strings.TrimSpace(v) }},
		{key: "BUTLER_GUARDED_TAB_METADATA_URL", component: "orchestrator", typeName: "string", requiresRestart: true, validate: validateGuardedMetadataURL, assign: func(v string) { cfg.GuardedTabMetadataURL = strings.TrimSpace(v) }},
		{key: "BUTLER_GUARDED_TAB_TOKEN_FILE", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(v string) { cfg.GuardedTabTokenFile = strings.TrimSpace(v) }},
	}
}

func validateGuardedMetadataURL(v string) error {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && (u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback())) {
		return fmt.Errorf("fixed private guarded metadata destination required")
	}
	return nil
}

func validateGuardedTabConfig(cfg OrchestratorConfig) error {
	if !cfg.GuardedTabOwnerEnabled {
		return nil
	}
	if cfg.GuardedTabHost == "" || len(cfg.GuardedTabHost) > 128 || cfg.GuardedTabMetadataURL == "" || cfg.GuardedTabTokenFile == "" || cfg.OwnerControlOwnerID == "" || (cfg.OwnerControlToken == "" && cfg.OwnerControlTokenFile == "") {
		return fmt.Errorf("guarded tab owner controls require private metadata, fixed host and configured owner authentication")
	}
	return nil
}
