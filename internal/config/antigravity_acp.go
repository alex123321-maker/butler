package config

func antigravityACPSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_MEMORY_MODEL_PROVIDER", component: "orchestrator", typeName: "string", allowedValues: []string{"", "antigravity", "antigravity-acp"}, requiresRestart: true, assign: func(value string) { cfg.MemoryModelProvider = value }},
		{key: "BUTLER_ANTIGRAVITY_ACP_SOCKET", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(value string) { cfg.AntigravityACPSocket = value }},
		{key: "BUTLER_ANTIGRAVITY_ACP_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(value string) { cfg.AntigravityACPEnabled = value == "true" }},
		{key: "BUTLER_ANTIGRAVITY_ACP_MODEL", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(value string) { cfg.AntigravityACPModel = value }},
		{key: "BUTLER_ANTIGRAVITY_ACP_EXECUTABLE", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(value string) { cfg.AntigravityACPExecutable = value }},
		{key: "BUTLER_ANTIGRAVITY_ACP_SHA256", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(value string) { cfg.AntigravityACPSHA256 = value }},
		{key: "BUTLER_ANTIGRAVITY_ACP_PROFILE_DIR", component: "orchestrator", typeName: "string", requiresRestart: true, assign: func(value string) { cfg.AntigravityACPProfileDir = value }},
	}
}
