package config

import "github.com/butler/butler/internal/modelprovider"

type SettingCatalogItem struct {
	Spec         fieldSpec
	Group        string
	DisplayOrder int
	Visible      bool
}

func managedSettingsCatalog() []SettingCatalogItem {
	return []SettingCatalogItem{
		{
			Spec:         fieldSpec{key: "BUTLER_LOG_LEVEL", description: "Уровень детализации логов оркестратора.", valueHint: "`debug` для диагностики, `info` для обычной работы, `warn` и `error` для более тихого продакшена.", component: "orchestrator", typeName: "string", required: false, defaultValue: "info", allowedValues: []string{"debug", "info", "warn", "error"}, requiresRestart: false, assign: nil},
			Group:        "General",
			DisplayOrder: 10,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_ENVIRONMENT", description: "Режим окружения Butler.", valueHint: "`development` для локальной разработки, `test` для тестов, `production` для постоянного развертывания.", component: "orchestrator", typeName: "string", required: false, defaultValue: "development", allowedValues: []string{"development", "test", "production"}, requiresRestart: false, assign: nil},
			Group:        "General",
			DisplayOrder: 20,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MODEL_PROVIDER", description: "Основной провайдер модели для новых run.", valueHint: "Выберите провайдера, через которого Butler будет выполнять запросы по умолчанию.", component: "orchestrator", typeName: "string", required: false, defaultValue: modelprovider.ProviderOpenAI, allowedValues: modelprovider.SupportedProviders(), requiresRestart: true},
			Group:        "Model",
			DisplayOrder: 25,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_API_KEY", description: "API-ключ OpenAI для прямого доступа к моделям и embeddings.", valueHint: "Оставьте пустым, если используете другой провайдер или OAuth-поток Codex.", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 30,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_MODEL", description: "Модель OpenAI для обычных агентных run.", valueHint: "Укажите точное имя модели, доступной вашему аккаунту.", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 40,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_TRANSPORT_MODE", description: "Предпочитаемый режим транспорта OpenAI.", valueHint: "`ws-first` сначала пробует WebSocket, `sse-only` использует только HTTP streaming.", component: "orchestrator", typeName: "string", required: false, defaultValue: "ws-first", allowedValues: []string{"ws-first", "sse-only"}, requiresRestart: true},
			Group:        "Model",
			DisplayOrder: 50,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_CODEX_MODEL", description: "Модель Codex для задач, ориентированных на код.", valueHint: "Должна совпадать с моделью, доступной в вашем Codex/OpenAI окружении.", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-5.1-codex", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 55,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_OPENAI_CODEX_BASE_URL", component: "orchestrator", typeName: "string", required: false, defaultValue: "https://chatgpt.com/backend-api", requiresRestart: true, validate: validateNonEmptyURL, assign: nil},
			Group:        "Model",
			DisplayOrder: 56,
			Visible:      false,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_GITHUB_COPILOT_MODEL", description: "Модель, используемая при работе через GitHub Copilot provider.", valueHint: "Укажите имя модели, доступной вашему Copilot-токену.", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Model",
			DisplayOrder: 57,
			Visible:      true,
		},
		{
			Spec:  fieldSpec{key: "BUTLER_ANTIGRAVITY_MODEL", component: "orchestrator", typeName: "string", defaultValue: "gemini-3.8-flash-tiered", requiresRestart: true, validate: validateNonEmpty},
			Group: "Model", DisplayOrder: 58, Visible: true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TELEGRAM_BOT_TOKEN", description: "Токен Telegram-бота для канала Telegram.", valueHint: "Получается у BotFather; без него Telegram-адаптер не сможет принимать и отправлять сообщения.", component: "orchestrator", typeName: "string", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmpty, assign: nil},
			Group:        "Telegram",
			DisplayOrder: 60,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TELEGRAM_ALLOWED_CHAT_IDS", description: "Список chat id, которым разрешено общаться с Butler в Telegram.", valueHint: "Несколько значений указываются через запятую, например `12345,67890`.", component: "orchestrator", typeName: "csv", required: false, defaultValue: "", requiresRestart: true, validate: validateOptionalChatIDList, assign: nil},
			Group:        "Telegram",
			DisplayOrder: 70,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_EXTENSION_API_TOKENS", description: "Токены доступа для browser extension и remote single-tab relay.", valueHint: "Укажите один или несколько токенов через запятую и скопируйте один из них в extension popup.", component: "orchestrator", typeName: "csv", required: false, defaultValue: "", isSecret: true, requiresRestart: true, validate: validateOptionalNonEmptyTokenList, assign: nil},
			Group:        "Browser Control",
			DisplayOrder: 74,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_SINGLE_TAB_TRANSPORT_MODE", description: "Режим доставки single-tab команд до браузера.", valueHint: "`native_only` — только локальный путь, `dual` — гибридный режим, `remote_preferred` — сначала remote relay.", component: "orchestrator", typeName: "string", required: false, defaultValue: "dual", allowedValues: []string{"native_only", "dual", "remote_preferred"}, requiresRestart: true, assign: nil},
			Group:        "Browser Control",
			DisplayOrder: 75,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS", description: "Через сколько секунд heartbeat remote extension считается устаревшим.", valueHint: "Чем меньше значение, тем быстрее Butler замечает отключённый браузер, но тем выше чувствительность к временным сбоям связи.", component: "orchestrator", typeName: "int", required: false, defaultValue: "90", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Browser Control",
			DisplayOrder: 76,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_BROWSER_LOCAL_ROLLOUT_MODE", description: "Режим rollout для локального browser runtime.", valueHint: "Обычно держите согласованным с `BUTLER_SINGLE_TAB_TRANSPORT_MODE`.", component: "tool-browser-local", typeName: "string", required: false, defaultValue: "native_only", allowedValues: []string{"native_only", "dual", "remote_preferred"}, requiresRestart: true, assign: nil},
			Group:        "Browser Control",
			DisplayOrder: 77,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE", description: "Канал диспетчеризации команд для локального browser runtime.", valueHint: "`browser_bridge` — прямой bridge, `orchestrator_relay` — проксирование через orchestrator relay.", component: "tool-browser-local", typeName: "string", required: false, defaultValue: "browser_bridge", allowedValues: []string{"browser_bridge", "orchestrator_relay"}, requiresRestart: true, assign: nil},
			Group:        "Browser Control",
			DisplayOrder: 78,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_PROFILE_LIMIT", description: "Сколько profile memory записей загружать в контекст run.", valueHint: "Больше значение даёт больше персонального контекста, но увеличивает размер prompt.", component: "orchestrator", typeName: "int", required: false, defaultValue: "20", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 80,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_EPISODIC_LIMIT", description: "Сколько episodic memory записей подтягивать в run.", valueHint: "Увеличивайте, если важны недавние похожие эпизоды; уменьшайте при риске раздувания контекста.", component: "orchestrator", typeName: "int", required: false, defaultValue: "3", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 90,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_SCOPE_ORDER", description: "Порядок поиска memory scope при сборке контекста.", valueHint: "Обычно `session,user,global`: сначала память сессии, затем пользователя, затем глобальная.", component: "orchestrator", typeName: "csv", required: false, defaultValue: "session,user,global", requiresRestart: true, validate: validateMemoryScopeOrder, assign: nil},
			Group:        "Memory",
			DisplayOrder: 100,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_WORKING_TRANSIENT_TTL_SECONDS", description: "TTL временного working state в Redis.", valueHint: "Задаёт, сколько секунд хранится краткоживущая рабочая память между шагами и повторами.", component: "orchestrator", typeName: "int", required: false, defaultValue: "1800", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 105,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_EMBEDDING_MODEL", description: "Модель для embeddings, используемых в semantic retrieval.", valueHint: "Должна соответствовать выбранному embedding provider.", component: "orchestrator", typeName: "string", required: false, defaultValue: "text-embedding-3-small", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Memory",
			DisplayOrder: 106,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_PIPELINE_ENABLED", description: "Включает асинхронный memory pipeline после завершения run.", valueHint: "`true` — сохранять и обогащать память автоматически, `false` — отключить pipeline.", component: "orchestrator", typeName: "string", required: false, defaultValue: "true", allowedValues: []string{"true", "false"}, requiresRestart: true},
			Group:        "Memory",
			DisplayOrder: 107,
			Visible:      true,
		},
		{
			Spec:  fieldSpec{key: "BUTLER_CONTEXT_BUDGET_TOKENS", component: "orchestrator", typeName: "int", defaultValue: "65536", requiresRestart: true, validate: validatePositiveInt},
			Group: "Memory", DisplayOrder: 107, Visible: true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_PIPELINE_POLL_TIMEOUT_SECONDS", component: "orchestrator", typeName: "int", required: false, defaultValue: "5", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 108,
			Visible:      false,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_PIPELINE_MAX_RETRIES", component: "orchestrator", typeName: "int", required: false, defaultValue: "3", requiresRestart: true, validate: validatePositiveInt, assign: nil},
			Group:        "Memory",
			DisplayOrder: 109,
			Visible:      false,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_MEMORY_EXTRACTION_MODEL", description: "Модель для извлечения memory candidates после run.", valueHint: "Можно выбрать более дешёвую модель, если extraction должен быть экономичнее основного run.", component: "orchestrator", typeName: "string", required: false, defaultValue: "gpt-4o-mini", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Memory",
			DisplayOrder: 110,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_DEFAULT_TARGET", description: "Целевой runtime tool-broker по умолчанию.", valueHint: "Обычно `local`, если инструменты исполняются в локальных контейнерах Butler.", component: "tool-broker", typeName: "string", required: false, defaultValue: "local", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Tools",
			DisplayOrder: 200,
			Visible:      true,
		},
		{
			Spec:         fieldSpec{key: "BUTLER_TOOL_REGISTRY_PATH", component: "tool-broker", typeName: "string", required: false, defaultValue: "configs/tools.json", requiresRestart: true, validate: validateNonEmpty, assign: nil},
			Group:        "Tools",
			DisplayOrder: 210,
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
