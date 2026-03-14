package config

import (
	"strings"
	"testing"
)

func TestLoadOrchestratorFromEnvUsesDefaultsAndEnvOverrides(t *testing.T) {
	get := envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://butler:secret@localhost:5432/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
		"BUTLER_LOG_LEVEL":    "warn",
		"BUTLER_HTTP_ADDR":    "127.0.0.1:8088",
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
	if cfg.OpenAIModel != "gpt-5-mini" {
		t.Fatalf("expected default OpenAI model, got %q", cfg.OpenAIModel)
	}
	if cfg.SessionLeaseTTLSeconds != 60 {
		t.Fatalf("expected default session lease ttl, got %d", cfg.SessionLeaseTTLSeconds)
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
	if model.DefaultValue != "gpt-5-mini" {
		t.Fatalf("expected default value to be recorded, got %q", model.DefaultValue)
	}

	leaseTTL := findKey(t, keys, "BUTLER_SESSION_LEASE_TTL_SECONDS")
	if leaseTTL.DefaultValue != "60" {
		t.Fatalf("expected session lease ttl default, got %q", leaseTTL.DefaultValue)
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
