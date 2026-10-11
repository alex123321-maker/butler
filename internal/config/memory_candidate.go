package config

import "github.com/butler/butler/internal/memory/candidatecontrol"

func memoryCandidateOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_MEMORY_CANDIDATE_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryCandidateEnabled = v == "true" }}}
}
func memoryCandidateBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_MEMORY_CANDIDATE_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.MemoryCandidateEnabled = v == "true" }},
		{key: "BUTLER_MEMORY_CANDIDATE_CONTROL_URL", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error { return validateOperatorControlEndpoint(v, candidatecontrol.Path) }, assign: func(v string) { cfg.MemoryCandidateControlURL = v }},
	}
}
