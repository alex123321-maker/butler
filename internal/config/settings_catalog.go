package config

type SettingCatalogItem struct {
	Spec         fieldSpec
	Group        string
	DisplayOrder int
	Visible      bool
}

func managedSettingsCatalog() []SettingCatalogItem {
	return []SettingCatalogItem{
		{
			Spec:         fieldSpec{key: "BUTLER_LOG_LEVEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "info", allowedValues: []string{"debug", "info", "warn", "error"}, requiresRestart: false, assign: nil},
			Group:        "General",
			DisplayOrder: 10,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_ENVIRONMENT", component: "orchestrator", typeName: "string", required: false, defaultValue: "development", allowedValues: []string{"development", "test", "production"}, requiresRestart: false, assign: nil},
			Group:        "General",
			DisplayOrder: 20,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_API_KEY", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 30,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_MODEL", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 40,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_TRANSPORT_MODE", component: "orchestrator", typeName: "string", required: false, defaultValue: "ws-first", allowedValues: []string{"ws-first", "sse-only"}, requiresRestart: true},
			Group:        "Model",
			DisplayOrder: 50,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TELEGRAM_BOT_TOKEN", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: nil},
			Group:        "Telegram",
			DisplayOrder: 60,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TELEGRAM_ALLOWED_CHAT_IDS", component: "orchestrator", typeName: "csv", required: false, defaultValue: "", requiresRestart: true, validate: validateOptionalChatIDList, assign: nil},
			Group:        "Telegram",
			DisplayOrder: 70,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_PROFILE_LIMIT", component: "orchestrator", typeName: "int", required: false, defaultValue: "20", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 80,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_EPISODIC_LIMIT", component: "orchestrator", typeName: "int", required: false, defaultValue: "3", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 90,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_SCOPE_ORDER", component: "orchestrator", typeName: "csv", required: false, defaultValue: "session,user,global", requiresRestart: true, validate: validateMemoryScopeOrder, assign: nil},
			Group:        "Memory",
			DisplayOrder: 100,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_DEFAULT_TARGET", component: "tool-broker", typeName: "string", required: false, defaultValue: "local", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Tools",
			DisplayOrder: 110,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_REGISTRY_PATH", component: "tool-broker", typeName: "string", required: false, defaultValue: "configs/tools.json", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Tools",
			DisplayOrder: 120,
			Visible:      false,
		},
	}
}

func managedSettingSpecByKey(key string) (SettingCatalogItem, bool) {
	for _, item := range managedSettingsCatalog() {
		if item.Spec.key == key {
			return item, true
		}
	}
	return SettingCatalogItem{}, false
}

func managedFieldSpecs() []fieldSpec {
	catalog := managedSettingsCatalog()
	specs := make([]fieldSpec, 0, len(catalog))
	for _, item := range catalog {
		specs = append(specs, item.Spec)
	}
	return specs
}
