package config

import (
	"strings"
	"testing"
)

func TestLoadOrchestratorFromEnvUsesDefaultsAndEnvOverrides(t *testing.T) {
	get := envMap(map[string]string{
		"BUTLER_POSTGRES_URL":         "postgres://butler:secret@localhost:5432/butler",
		"BUTLER_REDIS_URL":            "redis://localhost:6379/0",
		"BUTLER_LOG_LEVEL":            "warn",
		"BUTLER_HTTP_ADDR":            "127.0.0.1:8088",
		"BUTLER_OPENAI_API_KEY":       "sk-test",
		"BUTLER_EXTENSION_API_TOKENS": "ext-token-a,ext-token-b",
	})

	cfg, snapshot, err := loadOrchestrator(get)
	if err != nil {
		t.Fatalf("loadOrchestrator returned error: %v", err)
	}

	if cfg.Shared.ServiceName != "orchestrator" {
		t.Fatalf("expected default service name, got %q", cfg.Shared.ServiceName)
	}
	if cfg.Shared.LogLevel != "warn" {
		t.Fatalf("expected log level override, got %q", cfg.Shared.LogLevel)
	}
	if cfg.Shared.HTTPAddr != "127.0.0.1:8088" {
		t.Fatalf("expected HTTP addr override, got %q", cfg.Shared.HTTPAddr)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Fatalf("expected default OpenAI model, got %q", cfg.OpenAIModel)
	}
	if cfg.ModelProvider != "openai" {
		t.Fatalf("expected default model provider, got %q", cfg.ModelProvider)
	}
	if cfg.OpenAICodexModel != "gpt-5.1-codex" {
		t.Fatalf("expected default OpenAI Codex model, got %q", cfg.OpenAICodexModel)
	}
	if cfg.GitHubCopilotModel != "gpt-4o" {
		t.Fatalf("expected default GitHub Copilot model, got %q", cfg.GitHubCopilotModel)
	}
	if cfg.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected default OpenAI base URL, got %q", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIRealtimeURL != "wss://api.openai.com/v1/realtime" {
		t.Fatalf("expected default OpenAI realtime URL, got %q", cfg.OpenAIRealtimeURL)
	}
	if cfg.OpenAITransportMode != "ws-first" {
		t.Fatalf("expected default OpenAI transport mode, got %q", cfg.OpenAITransportMode)
	}
	if cfg.OpenAITimeoutSeconds != 60 {
		t.Fatalf("expected default OpenAI timeout, got %d", cfg.OpenAITimeoutSeconds)
	}
	if cfg.ToolBrokerAddr != "127.0.0.1:10090" {
		t.Fatalf("expected default tool broker addr, got %q", cfg.ToolBrokerAddr)
	}
	if cfg.TelegramBaseURL != "https://api.telegram.org" {
		t.Fatalf("expected default Telegram base URL, got %q", cfg.TelegramBaseURL)
	}
	if cfg.TelegramPollTimeout != 25 {
		t.Fatalf("expected default Telegram poll timeout, got %d", cfg.TelegramPollTimeout)
	}
	if cfg.SessionLeaseTTLSeconds != 60 {
		t.Fatalf("expected default session lease ttl, got %d", cfg.SessionLeaseTTLSeconds)
	}
	if cfg.MemoryProfileLimit != 20 {
		t.Fatalf("expected default profile limit, got %d", cfg.MemoryProfileLimit)
	}
	if cfg.MemoryEpisodicLimit != 3 {
		t.Fatalf("expected default episodic limit, got %d", cfg.MemoryEpisodicLimit)
	}
	if strings.Join(cfg.MemoryScopeOrder, ",") != "session,user,global" {
		t.Fatalf("unexpected memory scope order: %v", cfg.MemoryScopeOrder)
	}
	if cfg.MemoryWorkingTransientTTLSeconds != 1800 {
		t.Fatalf("expected default working transient ttl, got %d", cfg.MemoryWorkingTransientTTLSeconds)
	}
	if strings.Join(cfg.ExtensionAPITokens, ",") != "ext-token-a,ext-token-b" {
		t.Fatalf("expected extension api tokens override, got %v", cfg.ExtensionAPITokens)
	}
	if cfg.SingleTabTransportMode != "dual" {
		t.Fatalf("expected default single-tab transport mode, got %q", cfg.SingleTabTransportMode)
	}
	if cfg.SingleTabRelayHeartbeatTTLSeconds != 90 {
		t.Fatalf("expected default single-tab relay heartbeat ttl, got %d", cfg.SingleTabRelayHeartbeatTTLSeconds)
	}

	keys := snapshot.ListKeys()
	if len(keys) == 0 {
		t.Fatal("expected introspection keys")
	}

	postgres := findKey(t, keys, "BUTLER_POSTGRES_URL")
	if postgres.Source != "env" {
		t.Fatalf("expected env source, got %q", postgres.Source)
	}
	if postgres.EffectiveValue != "[masked]" {
		t.Fatalf("expected masked effective value, got %q", postgres.EffectiveValue)
	}
	if postgres.ValidationStatus != ValidationStatusValid {
		t.Fatalf("expected valid postgres key, got %q", postgres.ValidationStatus)
	}

	model := findKey(t, keys, "BUTLER_OPENAI_MODEL")
	if model.Source != "default" {
		t.Fatalf("expected default source, got %q", model.Source)
	}
	if model.DefaultValue != "gpt-4o-mini" {
		t.Fatalf("expected default value to be recorded, got %q", model.DefaultValue)
	}

	apiKey := findKey(t, keys, "BUTLER_OPENAI_API_KEY")
	if apiKey.EffectiveValue != "[masked]" {
		t.Fatalf("expected masked OpenAI API key, got %q", apiKey.EffectiveValue)
	}
	extensionTokens := findKey(t, keys, "BUTLER_EXTENSION_API_TOKENS")
	if extensionTokens.EffectiveValue != "[masked]" {
		t.Fatalf("expected masked extension API tokens, got %q", extensionTokens.EffectiveValue)
	}
	singleTabTransportMode := findKey(t, keys, "BUTLER_SINGLE_TAB_TRANSPORT_MODE")
	if singleTabTransportMode.DefaultValue != "dual" {
		t.Fatalf("expected single-tab transport mode default, got %q", singleTabTransportMode.DefaultValue)
	}
	heartbeatTTL := findKey(t, keys, "BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS")
	if heartbeatTTL.DefaultValue != "90" {
		t.Fatalf("expected relay heartbeat ttl default, got %q", heartbeatTTL.DefaultValue)
	}

	provider := findKey(t, keys, "BUTLER_MODEL_PROVIDER")
	if provider.DefaultValue != "openai" {
		t.Fatalf("expected default model provider, got %q", provider.DefaultValue)
	}

	baseURL := findKey(t, keys, "BUTLER_OPENAI_BASE_URL")
	if baseURL.DefaultValue != "https://api.openai.com/v1" {
		t.Fatalf("expected default OpenAI base URL, got %q", baseURL.DefaultValue)
	}

	realtimeURL := findKey(t, keys, "BUTLER_OPENAI_REALTIME_URL")
	if realtimeURL.DefaultValue != "wss://api.openai.com/v1/realtime" {
		t.Fatalf("expected default OpenAI realtime URL, got %q", realtimeURL.DefaultValue)
	}

	transportMode := findKey(t, keys, "BUTLER_OPENAI_TRANSPORT_MODE")
	if transportMode.DefaultValue != "ws-first" {
		t.Fatalf("expected default OpenAI transport mode, got %q", transportMode.DefaultValue)
	}

	toolBrokerAddr := findKey(t, keys, "BUTLER_TOOL_BROKER_ADDR")
	if toolBrokerAddr.DefaultValue != "127.0.0.1:10090" {
		t.Fatalf("expected default tool broker addr, got %q", toolBrokerAddr.DefaultValue)
	}

	telegramBaseURL := findKey(t, keys, "BUTLER_TELEGRAM_BASE_URL")
	if telegramBaseURL.DefaultValue != "https://api.telegram.org" {
		t.Fatalf("expected default Telegram base URL, got %q", telegramBaseURL.DefaultValue)
	}

	leaseTTL := findKey(t, keys, "BUTLER_SESSION_LEASE_TTL_SECONDS")
	if leaseTTL.DefaultValue != "60" {
		t.Fatalf("expected session lease ttl default, got %q", leaseTTL.DefaultValue)
	}

	memoryProfileLimit := findKey(t, keys, "BUTLER_MEMORY_PROFILE_LIMIT")
	if memoryProfileLimit.DefaultValue != "20" {
		t.Fatalf("expected memory profile limit default, got %q", memoryProfileLimit.DefaultValue)
	}

	memoryScopeOrder := findKey(t, keys, "BUTLER_MEMORY_SCOPE_ORDER")
	if memoryScopeOrder.DefaultValue != "session,user,global" {
		t.Fatalf("expected memory scope order default, got %q", memoryScopeOrder.DefaultValue)
	}

	transientTTL := findKey(t, keys, "BUTLER_MEMORY_WORKING_TRANSIENT_TTL_SECONDS")
	if transientTTL.DefaultValue != "1800" {
		t.Fatalf("expected working transient ttl default, got %q", transientTTL.DefaultValue)
	}
}

func TestLoadOrchestratorFromEnvValidatesRequiredFields(t *testing.T) {
	_, snapshot, err := loadOrchestrator(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("expected missing required field error")
	}

	if !strings.Contains(err.Error(), "BUTLER_POSTGRES_URL") || !strings.Contains(err.Error(), "BUTLER_REDIS_URL") {
		t.Fatalf("expected both required keys in error, got %v", err)
	}

	postgres := findKey(t, snapshot.ListKeys(), "BUTLER_POSTGRES_URL")
	if postgres.ValidationStatus != ValidationStatusMissing {
		t.Fatalf("expected missing status, got %q", postgres.ValidationStatus)
	}
	if postgres.ValidationError == "" {
		t.Fatal("expected validation error for missing field")
	}
}

func TestLoadToolBrokerValidatesAllowedValues(t *testing.T) {
	_, snapshot, err := loadToolBroker(envMap(map[string]string{
		"BUTLER_LOG_LEVEL": "verbose",
	}))
	if err == nil {
		t.Fatal("expected invalid log level error")
	}

	key := findKey(t, snapshot.ListKeys(), "BUTLER_LOG_LEVEL")
	if key.ValidationStatus != ValidationStatusInvalid {
		t.Fatalf("expected invalid status, got %q", key.ValidationStatus)
	}
	if !strings.Contains(key.ValidationError, "must be one of") {
		t.Fatalf("expected allowed values error, got %q", key.ValidationError)
	}
}

func TestLoadOrchestratorRequiresAllowedChatsWhenTelegramEnabled(t *testing.T) {
	_, _, err := loadOrchestrator(envMap(map[string]string{
		"BUTLER_POSTGRES_URL":       "postgres://butler:secret@localhost:5432/butler",
		"BUTLER_REDIS_URL":          "redis://localhost:6379/0",
		"BUTLER_TELEGRAM_BOT_TOKEN": "telegram-token",
	}))
	if err == nil {
		t.Fatal("expected telegram allowed chat ids validation error")
	}
	if !strings.Contains(err.Error(), "BUTLER_TELEGRAM_ALLOWED_CHAT_IDS") {
		t.Fatalf("expected allowed chat ids error, got %v", err)
	}
}

func TestLoadOrchestratorFromEnvTreatsBlankEnvironmentValuesAsUnset(t *testing.T) {
	cfg, snapshot, err := loadOrchestrator(envMap(map[string]string{
		"BUTLER_POSTGRES_URL":              "postgres://butler:secret@localhost:5432/butler",
		"BUTLER_REDIS_URL":                 "redis://localhost:6379/0",
		"BUTLER_TELEGRAM_BOT_TOKEN":        "   ",
		"BUTLER_TELEGRAM_ALLOWED_CHAT_IDS": "",
	}))
	if err != nil {
		t.Fatalf("loadOrchestrator returned error: %v", err)
	}
	if cfg.TelegramBotToken != "" {
		t.Fatalf("expected blank telegram token to be ignored, got %q", cfg.TelegramBotToken)
	}
	botToken := findKey(t, snapshot.ListKeys(), "BUTLER_TELEGRAM_BOT_TOKEN")
	if botToken.Source != ConfigSourceDefault {
		t.Fatalf("expected default source for blank env token, got %q", botToken.Source)
	}
}

func TestSnapshotListKeysReturnsCopy(t *testing.T) {
	_, snapshot, err := loadToolBroker(envMap(map[string]string{}))
	if err != nil {
		t.Fatalf("loadToolBroker returned error: %v", err)
	}

	keys := snapshot.ListKeys()
	keys[0].Key = "mutated"

	again := snapshot.ListKeys()
	if again[0].Key == "mutated" {
		t.Fatal("expected ListKeys to return a copy")
	}
}

func TestLoadToolBrowserFromEnvUsesSharedDefaults(t *testing.T) {
	cfg, snapshot, err := loadToolBrowser(envMap(map[string]string{}))
	if err != nil {
		t.Fatalf("loadToolBrowser returned error: %v", err)
	}
	if cfg.Shared.ServiceName != "tool-browser" {
		t.Fatalf("expected tool-browser service name, got %q", cfg.Shared.ServiceName)
	}
	if findKey(t, snapshot.ListKeys(), "BUTLER_HTTP_ADDR").DefaultValue != ":8080" {
		t.Fatal("expected shared HTTP addr default")
	}
	if cfg.NodeBinary != "node" {
		t.Fatalf("expected default node binary, got %q", cfg.NodeBinary)
	}
	if cfg.HelperScriptPath != "apps/tool-browser/scripts/browser_runtime.mjs" {
		t.Fatalf("unexpected default browser script path %q", cfg.HelperScriptPath)
	}
}

func TestLoadToolBrowserLocalFromEnvUsesDefaultsAndOverrides(t *testing.T) {
	cfg, snapshot, err := loadToolBrowserLocal(envMap(map[string]string{
		"BUTLER_TOOL_BROWSER_LOCAL_ORCHESTRATOR_URL": "http://localhost:28080",
		"BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE":    "orchestrator_relay",
		"BUTLER_TOOL_BROWSER_LOCAL_ROLLOUT_MODE":     "remote_preferred",
	}))
	if err != nil {
		t.Fatalf("loadToolBrowserLocal returned error: %v", err)
	}
	if cfg.Shared.ServiceName != "tool-browser-local" {
		t.Fatalf("expected tool-browser-local service name, got %q", cfg.Shared.ServiceName)
	}
	if cfg.OrchestratorBaseURL != "http://localhost:28080" {
		t.Fatalf("unexpected orchestrator URL %q", cfg.OrchestratorBaseURL)
	}
	if cfg.BrowserBridgeURL != "http://127.0.0.1:29115" {
		t.Fatalf("unexpected browser bridge URL %q", cfg.BrowserBridgeURL)
	}
	if cfg.DispatchMode != "orchestrator_relay" {
		t.Fatalf("unexpected dispatch mode %q", cfg.DispatchMode)
	}
	if cfg.DispatchRolloutMode != "remote_preferred" {
		t.Fatalf("unexpected dispatch rollout mode %q", cfg.DispatchRolloutMode)
	}
	if cfg.RequestTimeout != 15 {
		t.Fatalf("expected default request timeout 15, got %d", cfg.RequestTimeout)
	}
	key := findKey(t, snapshot.ListKeys(), "BUTLER_TOOL_BROWSER_LOCAL_ORCHESTRATOR_URL")
	if key.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %q", key.Source)
	}
	mode := findKey(t, snapshot.ListKeys(), "BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE")
	if mode.DefaultValue != "browser_bridge" {
		t.Fatalf("expected dispatch mode default, got %q", mode.DefaultValue)
	}
	rolloutMode := findKey(t, snapshot.ListKeys(), "BUTLER_TOOL_BROWSER_LOCAL_ROLLOUT_MODE")
	if rolloutMode.DefaultValue != "native_only" {
		t.Fatalf("expected rollout mode default, got %q", rolloutMode.DefaultValue)
	}
}

func TestLoadToolWebFetchFromEnvUsesDefaultsAndOverrides(t *testing.T) {
	cfg, snapshot, err := loadToolWebFetch(envMap(map[string]string{
		"BUTLER_WEBFETCH_SELF_HOSTED_BASE_URL": "http://crawl4ai:11235",
		"BUTLER_WEBFETCH_JINA_AUTH_TOKEN":      "jina-secret",
	}))
	if err != nil {
		t.Fatalf("loadToolWebFetch returned error: %v", err)
	}
	if cfg.Shared.ServiceName != "tool-webfetch" {
		t.Fatalf("expected tool-webfetch service name, got %q", cfg.Shared.ServiceName)
	}
	if cfg.SelfHostedBaseURL != "http://crawl4ai:11235" {
		t.Fatalf("unexpected self-hosted base URL %q", cfg.SelfHostedBaseURL)
	}
	if cfg.JinaBaseURL != "" {
		t.Fatalf("expected empty default Jina base URL, got %q", cfg.JinaBaseURL)
	}
	if cfg.JinaAuthToken != "jina-secret" {
		t.Fatalf("expected Jina auth token override, got %q", cfg.JinaAuthToken)
	}
	if !cfg.PlainHTTPEnabled {
		t.Fatal("expected plain HTTP fallback to default to enabled")
	}

	keys := snapshot.ListKeys()
	jinaToken := findKey(t, keys, "BUTLER_WEBFETCH_JINA_AUTH_TOKEN")
	if jinaToken.EffectiveValue != "[masked]" {
		t.Fatalf("expected masked Jina auth token, got %q", jinaToken.EffectiveValue)
	}
	plainHTTP := findKey(t, keys, "BUTLER_WEBFETCH_PLAIN_HTTP_ENABLED")
	if plainHTTP.DefaultValue != "true" {
		t.Fatalf("expected plain HTTP default to be recorded, got %q", plainHTTP.DefaultValue)
	}
}

func TestLoadBrowserBridgeFromEnvUsesDefaultsAndOverrides(t *testing.T) {
	cfg, snapshot, err := loadBrowserBridge(envMap(map[string]string{
		"BUTLER_BROWSER_BRIDGE_ORCHESTRATOR_URL": "http://localhost:18080",
	}))
	if err != nil {
		t.Fatalf("loadBrowserBridge returned error: %v", err)
	}
	if cfg.Shared.ServiceName != "browser-bridge" {
		t.Fatalf("expected browser-bridge service name, got %q", cfg.Shared.ServiceName)
	}
	if cfg.OrchestratorBaseURL != "http://localhost:18080" {
		t.Fatalf("unexpected orchestrator base URL %q", cfg.OrchestratorBaseURL)
	}
	if cfg.ControlAddr != "127.0.0.1:29115" {
		t.Fatalf("unexpected browser bridge control addr %q", cfg.ControlAddr)
	}
	if cfg.RequestTimeoutSeconds != 15 {
		t.Fatalf("expected default request timeout, got %d", cfg.RequestTimeoutSeconds)
	}
	key := findKey(t, snapshot.ListKeys(), "BUTLER_BROWSER_BRIDGE_ORCHESTRATOR_URL")
	if key.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %q", key.Source)
	}
}

func TestLoadRestartHelperFromEnvUsesAllowlistConfig(t *testing.T) {
	cfg, snapshot, err := loadRestartHelper(envMap(map[string]string{
		"BUTLER_RESTART_ALLOWED_SERVICES": "orchestrator,web,tool-broker",
	}))
	if err != nil {
		t.Fatalf("loadRestartHelper returned error: %v", err)
	}
	if cfg.Shared.ServiceName != "restart-helper" {
		t.Fatalf("expected restart-helper service name, got %q", cfg.Shared.ServiceName)
	}
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("unexpected docker host %q", cfg.DockerHost)
	}
	if strings.Join(cfg.AllowedServices, ",") != "orchestrator,web,tool-broker" {
		t.Fatalf("unexpected allowed services %v", cfg.AllowedServices)
	}
	if cfg.SelfService != "restart-helper" {
		t.Fatalf("unexpected self service %q", cfg.SelfService)
	}
	allowed := findKey(t, snapshot.ListKeys(), "BUTLER_RESTART_ALLOWED_SERVICES")
	if allowed.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %q", allowed.Source)
	}
}

func TestLoadToolDoctorParsesContainerTargets(t *testing.T) {
	cfg, snapshot, err := loadToolDoctor(envMap(map[string]string{
		"BUTLER_DOCTOR_CONTAINER_TARGETS": "orchestrator=http://orchestrator:8080/health,tool-broker=http://tool-broker:8080/health",
		"BUTLER_OPENAI_API_KEY":           "sk-test",
	}))
	if err != nil {
		t.Fatalf("loadToolDoctor returned error: %v", err)
	}
	if len(cfg.ContainerTargets) != 2 {
		t.Fatalf("expected container targets to parse, got %+v", cfg.ContainerTargets)
	}
	if cfg.OpenAIAPIKey != "sk-test" {
		t.Fatalf("expected OpenAI API key override, got %q", cfg.OpenAIAPIKey)
	}
	key := findKey(t, snapshot.ListKeys(), "BUTLER_DOCTOR_CONTAINER_TARGETS")
	if key.DefaultValue != "" {
		t.Fatalf("expected empty default for doctor targets, got %q", key.DefaultValue)
	}
}

func envMap(values map[string]string) envGetter {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func findKey(t *testing.T, keys []ConfigKeyInfo, key string) ConfigKeyInfo {
	t.Helper()
	for _, item := range keys {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("key %q not found", key)
	return ConfigKeyInfo{}
}
