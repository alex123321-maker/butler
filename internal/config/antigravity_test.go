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

func TestAntigravityOutputCeilingEnvironmentAndLayeredConfiguration(t *testing.T) {
	for _, raw := range []string{"0", "32768", "-1", "32769", "invalid"} {
		get := envMap(map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/butler", "BUTLER_REDIS_URL": "redis://localhost/0", "BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS": raw})
		cfg, _, err := loadOrchestrator(get)
		bad := raw == "-1" || raw == "32769" || raw == "invalid"
		if bad {
			if err == nil {
				t.Fatal("invalid ceiling accepted", raw)
			}
			continue
		}
		want := 0
		if raw == "32768" {
			want = 32768
		}
		if err != nil || cfg.AntigravityMaximumOutputTokens != want {
			t.Fatal("environment ceiling lost", raw, err)
		}
		layered, _, err := loadOrchestratorLayered(envMap(map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/butler", "BUTLER_REDIS_URL": "redis://localhost/0"}), []Setting{{Key: "BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS", Value: raw, Component: "orchestrator"}})
		if err != nil || layered.AntigravityMaximumOutputTokens != want {
			t.Fatal("stored ceiling lost", raw, err)
		}
	}
}
