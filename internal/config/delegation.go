package config

import (
	"fmt"
	"strings"
)

func delegationOrchestratorSpecs(cfg *OrchestratorConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_DELEGATION_ENABLED", component: "orchestrator", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.DelegationEnabled = v == "true" }},
		{key: "BUTLER_DELEGATION_MAX_PARALLEL", component: "orchestrator", typeName: "int", defaultValue: "1", requiresRestart: true, validate: func(v string) error {
			if v != "1" && v != "2" {
				return fmt.Errorf("delegation executor width must be one or two")
			}
			return nil
		}, assign: func(v string) { cfg.DelegationMaxParallel = mustParseInt(v) }},
	}
}

func delegationBrokerSpecs(cfg *ToolBrokerConfig) []fieldSpec {
	return []fieldSpec{
		{key: "BUTLER_DELEGATION_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.DelegationEnabled = v == "true" }},
		{key: "BUTLER_DELEGATION_CONTROL_URL", component: "tool-broker", typeName: "string", requiresRestart: true, validate: func(v string) error { return validateOperatorControlEndpoint(v, "/internal/delegation") }, assign: func(v string) { cfg.DelegationControlURL = strings.TrimSpace(v) }},
	}
}
