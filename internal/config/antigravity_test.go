package config

import "testing"

func TestAntigravityNativeConfigurationHasNoExecutableOrProfile(t *testing.T) {
	values := envMap(map[string]string{"BUTLER_POSTGRES_URL": "postgres://butler:password@localhost/butler", "BUTLER_REDIS_URL": "redis://localhost:6379/0", "BUTLER_MODEL_PROVIDER": "antigravity"})
	cfg, snapshot, err := loadOrchestrator(values)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AntigravityModel != "gemini-3.8-flash-tiered" {
		t.Fatalf("unexpected default model %s", cfg.AntigravityModel)
	}
	for _, item := range snapshot.ListKeys() {
		if item.Key == "BUTLER_ANTIGRAVITY_BINARY_PATH" || item.Key == "BUTLER_ANTIGRAVITY_PROFILE_DIR" {
			t.Fatal("CLI inference configuration survived")
		}
	}
	if _, editable := managedSettingSpecByKey("BUTLER_PROVIDER_AUTH_ANTIGRAVITY"); editable {
		t.Fatal("OAuth record exposed through settings")
	}
}
