package config

import "testing"

func TestTemporalMemoryRequiresOperatorOptIn(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_MEMORY_TEMPORAL_ENABLED", Value: "true"}})
	if err != nil || c.MemoryTemporalEnabled {
		t.Fatal("stored setting enabled temporal topology", err)
	}
	env["BUTLER_MEMORY_TEMPORAL_ENABLED"] = "true"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.MemoryTemporalEnabled {
		t.Fatal("operator opt-in ignored", err)
	}
}
