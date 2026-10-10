package config

import "testing"

func TestAntigravityConfigurationIsExplicitAndNonSecret(t *testing.T) {
	cfg, snapshot, err := loadOrchestrator(envMap(map[string]string{
		"BUTLER_POSTGRES_URL":            "postgres://butler:password@localhost/butler",
		"BUTLER_REDIS_URL":               "redis://localhost:6379/0",
		"BUTLER_MODEL_PROVIDER":          "antigravity",
		"BUTLER_ANTIGRAVITY_MODEL":       "gemini-3.8-flash-tiered",
		"BUTLER_ANTIGRAVITY_BINARY_PATH": "/opt/antigravity/agy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelProvider != "antigravity" || cfg.AntigravityModel != "gemini-3.8-flash-tiered" || cfg.AntigravityProfileDir != "/home/butler" {
		t.Fatalf("incorrect config: %s %s %s", cfg.ModelProvider, cfg.AntigravityModel, cfg.AntigravityProfileDir)
	}
	for _, key := range snapshot.ListKeys() {
		if key.Key == "BUTLER_ANTIGRAVITY_MODEL" && key.IsSecret {
			t.Fatal("model slug marked secret")
		}
	}
	if _, editable := managedSettingSpecByKey("BUTLER_ANTIGRAVITY_BINARY_PATH"); editable {
		t.Fatal("runtime executable must not be editable through ordinary settings")
	}
	layered, _, err := loadOrchestratorLayered(envMap(map[string]string{
		"BUTLER_POSTGRES_URL": "postgres://butler:password@localhost/butler",
		"BUTLER_REDIS_URL":    "redis://localhost:6379/0",
	}), []Setting{{Key: "BUTLER_ANTIGRAVITY_BINARY_PATH", Value: "/untrusted/custom-binary"}, {Key: "BUTLER_ANTIGRAVITY_PROFILE_DIR", Value: "/another/profile"}})
	if err != nil {
		t.Fatal(err)
	}
	if layered.AntigravityBinaryPath != "/opt/antigravity/agy" || layered.AntigravityProfileDir != "/home/butler" {
		t.Fatal("database settings changed operational CLI boundaries")
	}
}
