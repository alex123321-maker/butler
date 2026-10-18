package config

import "github.com/butler/butler/internal/schedulecontrol"

func schedulesOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{{key: "BUTLER_SCHEDULES_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.SchedulesEnabled = v == "true" }}}
}
func schedulesBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_SCHEDULES_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.SchedulesEnabled = v == "true" }},
		{key: "BUTLER_SCHEDULES_CONTROL_URL", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error { return validateOperatorControlEndpoint(v, schedulecontrol.Path) }, assign: func(v string) { cfg.SchedulesControlURL = v }},
	}
}
