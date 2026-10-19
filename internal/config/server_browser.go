package config

import "strings"

func serverBrowserOrchestratorSpec(cfg *OrchestratorConfig) fieldSpec {
	return fieldSpec{key: "BUTLER_BROWSER_CONTROL_URL", component: "orchestrator", typeName: "string", requiresRestart: true,
		validate: func(v string) error {
			return validateOperatorControlEndpoint(strings.TrimSpace(v), "/internal/server-browser")
		},
		assign: func(v string) { cfg.BrowserControlURL = strings.TrimSpace(v) }}
}
