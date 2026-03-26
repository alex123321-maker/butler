package config

import "testing"

func TestLoadOrchestratorLayeredPrefersEnvironmentOverDatabase(t *testing.T) {
	settings := []Setting{
		{Key: "BUTLER_LOG_LEVEL", Value: "debug", Component: "orchestrator", UpdatedBy: "unit-test"},
		{Key: "BUTLER_OPENAI_MODEL", Value: "gpt-db", Component: "orchestrator", UpdatedBy: "unit-test"},
	}

	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
		"BUTLER_LOG_LEVEL":    "error",
	}), settings)
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if cfg.Shared.LogLevel != "error" {
		t.Fatalf("expected env log level to win, got %q", cfg.Shared.LogLevel)
	}
	if cfg.OpenAIModel != "gpt-db" {
		t.Fatalf("expected db OpenAI model to win over default, got %q", cfg.OpenAIModel)
	}

	logLevel := findKey(t, snapshot.ListKeys(), "BUTLER_LOG_LEVEL")
	if logLevel.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %q", logLevel.Source)
	}
	model := findKey(t, snapshot.ListKeys(), "BUTLER_OPENAI_MODEL")
	if model.Source != ConfigSourceDB {
		t.Fatalf("expected db source, got %q", model.Source)
	}
	baseURL := findKey(t, snapshot.ListKeys(), "BUTLER_OPENAI_BASE_URL")
	if baseURL.Source != ConfigSourceDefault {
		t.Fatalf("expected default source, got %q", baseURL.Source)
	}
}

func TestLoadOrchestratorLayeredValidatesDatabaseValues(t *testing.T) {
	_, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
	}), []Setting{{Key: "BUTLER_OPENAI_TIMEOUT_SECONDS", Value: "0", Component: "orchestrator", UpdatedBy: "unit-test"}})
	if err == nil {
		t.Fatal("expected validation error for invalid db value")
	}
	timeout := findKey(t, snapshot.ListKeys(), "BUTLER_OPENAI_TIMEOUT_SECONDS")
	if timeout.Source != ConfigSourceDB {
		t.Fatalf("expected db source, got %q", timeout.Source)
	}
	if timeout.ValidationStatus != ValidationStatusInvalid {
		t.Fatalf("expected invalid status, got %q", timeout.ValidationStatus)
	}
}

func TestLayeredResolverAllowsDatabaseBlankOverrides(t *testing.T) {
	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
	}), []Setting{{Key: "BUTLER_OPENAI_API_KEY", Value: "", Component: "orchestrator", IsSecret: true, UpdatedBy: "unit-test"}})
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if cfg.OpenAIAPIKey != "" {
		t.Fatalf("expected blank db override to apply, got %q", cfg.OpenAIAPIKey)
	}
	apiKey := findKey(t, snapshot.ListKeys(), "BUTLER_OPENAI_API_KEY")
	if apiKey.Source != ConfigSourceDB {
		t.Fatalf("expected db source, got %q", apiKey.Source)
	}
	if apiKey.EffectiveValue != "" {
		t.Fatalf("expected empty masked value, got %q", apiKey.EffectiveValue)
	}
}

func TestLoadOrchestratorLayeredTreatsBlankEnvironmentValuesAsUnset(t *testing.T) {
	settings := []Setting{{Key: "BUTLER_OPENAI_MODEL", Value: "gpt-db", Component: "orchestrator", UpdatedBy: "unit-test"}}

	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
		"BUTLER_OPENAI_MODEL": "   ",
	}), settings)
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if cfg.OpenAIModel != "gpt-db" {
		t.Fatalf("expected db model to win when env is blank, got %q", cfg.OpenAIModel)
	}
	model := findKey(t, snapshot.ListKeys(), "BUTLER_OPENAI_MODEL")
	if model.Source != ConfigSourceDB {
		t.Fatalf("expected db source, got %q", model.Source)
	}
}

func TestLoadOrchestratorLayeredMemoryPipelineDefaultsToTrue(t *testing.T) {
	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
	}), nil)
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if !cfg.MemoryPipelineEnabled {
		t.Fatal("expected MemoryPipelineEnabled to default to true")
	}
	pipelineKey := findKey(t, snapshot.ListKeys(), "BUTLER_MEMORY_PIPELINE_ENABLED")
	if pipelineKey.Source != ConfigSourceDefault {
		t.Fatalf("expected default source, got %q", pipelineKey.Source)
	}
	if pipelineKey.EffectiveValue != "true" {
		t.Fatalf("expected effective value 'true', got %q", pipelineKey.EffectiveValue)
	}
}

func TestLoadOrchestratorLayeredIncludesRestartHelperURL(t *testing.T) {
	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL":       "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":          "redis://localhost:6379/0",
		"BUTLER_RESTART_HELPER_URL": "http://restart-helper:18080",
	}), nil)
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if cfg.RestartHelperURL != "http://restart-helper:18080" {
		t.Fatalf("expected restart helper URL override, got %q", cfg.RestartHelperURL)
	}
	restartHelperURL := findKey(t, snapshot.ListKeys(), "BUTLER_RESTART_HELPER_URL")
	if restartHelperURL.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %q", restartHelperURL.Source)
	}
}

func TestLoadOrchestratorLayeredIncludesExtensionRelaySettings(t *testing.T) {
	settings := []Setting{
		{
			Key:       "BUTLER_SINGLE_TAB_TRANSPORT_MODE",
			Value:     "remote_preferred",
			Component: "orchestrator",
			UpdatedBy: "unit-test",
		},
		{
			Key:       "BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS",
			Value:     "120",
			Component: "orchestrator",
			UpdatedBy: "unit-test",
		},
	}

	cfg, snapshot, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL":         "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":            "redis://localhost:6379/0",
		"BUTLER_EXTENSION_API_TOKENS": "ext-token-a,ext-token-b",
	}), settings)
	if err != nil {
		t.Fatalf("loadOrchestratorLayered returned error: %v", err)
	}
	if cfg.SingleTabTransportMode != "remote_preferred" {
		t.Fatalf("expected db transport mode override, got %q", cfg.SingleTabTransportMode)
	}
	if cfg.SingleTabRelayHeartbeatTTLSeconds != 120 {
		t.Fatalf("expected db relay heartbeat ttl override, got %d", cfg.SingleTabRelayHeartbeatTTLSeconds)
	}
	if len(cfg.ExtensionAPITokens) != 2 || cfg.ExtensionAPITokens[0] != "ext-token-a" || cfg.ExtensionAPITokens[1] != "ext-token-b" {
		t.Fatalf("expected extension api tokens from env, got %v", cfg.ExtensionAPITokens)
	}

	transportMode := findKey(t, snapshot.ListKeys(), "BUTLER_SINGLE_TAB_TRANSPORT_MODE")
	if transportMode.Source != ConfigSourceDB {
		t.Fatalf("expected transport mode source db, got %q", transportMode.Source)
	}
	relayTTL := findKey(t, snapshot.ListKeys(), "BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS")
	if relayTTL.Source != ConfigSourceDB {
		t.Fatalf("expected relay ttl source db, got %q", relayTTL.Source)
	}
	extensionTokens := findKey(t, snapshot.ListKeys(), "BUTLER_EXTENSION_API_TOKENS")
	if extensionTokens.Source != ConfigSourceEnv {
		t.Fatalf("expected extension tokens source env, got %q", extensionTokens.Source)
	}
	if extensionTokens.EffectiveValue != "[masked]" {
		t.Fatalf("expected masked extension tokens value, got %q", extensionTokens.EffectiveValue)
	}
}
