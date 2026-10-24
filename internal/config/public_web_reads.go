package config

func publicWebReadsBrokerSpec(cfg *ToolBrokerConfig) fieldSpec {
	return fieldSpec{key: "BUTLER_PUBLIC_WEB_READS_ENABLED", component: "tool-broker", typeName: "bool", defaultValue: "false", allowedValues: []string{"true", "false"}, requiresRestart: true, assign: func(v string) { cfg.PublicWebReadsEnabled = v == "true" }}
}
