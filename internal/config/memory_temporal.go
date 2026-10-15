package config

func memoryTemporalSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_MEMORY_TEMPORAL_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryTemporalEnabled = v == "true" }}}
}
