package config

import "testing"

func TestSchedulesRequireOperatorComposition(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	cfg, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_SCHEDULES_ENABLED", Value: "true"}})
	if err != nil || cfg.SchedulesEnabled {
		t.Fatal("stored settings enabled schedules", err)
	}
	env["BUTLER_SCHEDULES_ENABLED"] = "true"
	if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
		t.Fatal("enabled without private authentication")
	}
	env["BUTLER_ACTION_AUTHORITY_TOKEN_FILE"] = "/private/token"
	cfg, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !cfg.SchedulesEnabled {
		t.Fatal("operator scheduling disabled", err)
	}
	for _, endpoint := range []string{"http://localhost:8080/other", "http://user:pass@localhost:8080/internal/schedules", "http://localhost:8080/internal/schedules?x=1", "http://localhost:8080/internal/schedules?"} {
		env["BUTLER_SCHEDULES_CONTROL_URL"] = endpoint
		if _, _, err = loadToolBroker(envMap(env)); err == nil {
			t.Fatal("unbounded endpoint accepted", endpoint)
		}
	}
}
