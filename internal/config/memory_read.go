package config

import "github.com/butler/butler/internal/memory/readcontrol"

func memoryReadOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_MEMORY_READ_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryReadEnabled = v == "true" }}}
}
func memoryReadBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_MEMORY_READ_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryReadEnabled = v == "true" }},
		{key: "BUTLER_MEMORY_READ_CONTROL_URL", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error { return validateOperatorControlEndpoint(v, readcontrol.Path) }, assign: func(v string) { cfg.MemoryReadControlURL = v }},
	}
}
