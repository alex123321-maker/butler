package config

func toolDiscoverySpec(component string, target *bool) fieldSpec {
	return fieldSpec{key: "BUTLER_TOOL_DISCOVERY_ENABLED", component: component, typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { *target = v == "true" }}
}
