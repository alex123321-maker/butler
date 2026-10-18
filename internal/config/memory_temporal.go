package config

import "github.com/butler/butler/internal/memory/factcontrol"

func memoryTemporalSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_MEMORY_TEMPORAL_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryTemporalEnabled = v == "true" }}}
}

func memoryTemporalBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_MEMORY_TEMPORAL_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryTemporalEnabled = v == "true" }},
		{key: "BUTLER_MEMORY_FACT_CONTROL_URL", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error { return validateOperatorControlEndpoint(v, factcontrol.Path) }, assign: func(v string) { cfg.MemoryFactControlURL = v }},
	}
}
