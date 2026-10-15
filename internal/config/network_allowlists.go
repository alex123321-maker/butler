package config

import "github.com/butler/butler/internal/security/egress"

func networkAllowlistBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_NETWORK_ALLOWLISTS_JSON", component: "tool-broker", typeName: "json", requiresRestart: true,
		validate: func(raw string) error { _, err := egress.ParseClassAllowlists(raw); return err },
		assign:   func(raw string) { cfg.NetworkAllowlists, _ = egress.ParseClassAllowlists(raw) },
	}}
}
