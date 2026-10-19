package config

import "testing"

func TestToolDiscoveryRequiresOperatorFlagAndAuthentication(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	cfg, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_TOOL_DISCOVERY_ENABLED", Value: "true"}})
	if err != nil || cfg.ToolDiscoveryEnabled {
		t.Fatal("stored setting enabled discovery", err)
	}
	env["BUTLER_TOOL_DISCOVERY_ENABLED"] = "true"
	if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
		t.Fatal("enabled without Broker auth")
	}
	env["BUTLER_ACTION_AUTHORITY_TOKEN_FILE"] = "/private/token"
	cfg, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !cfg.ToolDiscoveryEnabled {
		t.Fatal("operator discovery unavailable", err)
	}
}
