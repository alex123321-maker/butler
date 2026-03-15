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
