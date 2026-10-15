package config

import "testing"

func TestMemoryReadTopologyRequiresOperatorOptIn(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_MEMORY_READ_ENABLED", Value: "true"}})
	if err != nil || c.MemoryReadEnabled {
		t.Fatal("stored setting enabled private read endpoint", err)
	}
	env["BUTLER_MEMORY_READ_ENABLED"] = "true"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.MemoryReadEnabled {
		t.Fatal("operator opt-in ignored", err)
	}
	if c.MemoryAdaptiveRetrievalEnabled {
		t.Fatal("adaptive default enabled")
	}
	c, _, err = loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED", Value: "true"}})
	if err != nil || c.MemoryAdaptiveRetrievalEnabled {
		t.Fatal("stored adaptive flag enabled", err)
	}
	env["BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED"] = "true"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.MemoryAdaptiveRetrievalEnabled {
		t.Fatal("operator adaptive flag ignored", err)
	}
	for _, url := range []string{"http://localhost:8080/other", "http://user:private@127.0.0.1:8080/internal/memory-read", "http://127.0.0.1:8080/internal/memory-read?private=1"} {
		env["BUTLER_MEMORY_READ_CONTROL_URL"] = url
		if _, _, e := loadToolBroker(envMap(env)); e == nil {
			t.Fatal("arbitrary control endpoint", url)
		}
	}
}
