package config

import "testing"

func TestMemoryCandidateTopologyIsOperatorOnly(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_MEMORY_CANDIDATE_ENABLED", Value: "true"}})
	if err != nil || c.MemoryCandidateEnabled {
		t.Fatal("database enabled candidate publication", err)
	}
	env["BUTLER_MEMORY_CANDIDATE_ENABLED"] = "true"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.MemoryCandidateEnabled {
		t.Fatal("operator opt-in lost", err)
	}
	for _, endpoint := range []string{"http://localhost:8080/other", "http://user:private@localhost:8080/internal/memory-candidate", "http://localhost:8080/internal/memory-candidate?private=1"} {
		env["BUTLER_MEMORY_CANDIDATE_CONTROL_URL"] = endpoint
		if _, _, err = loadToolBroker(envMap(env)); err == nil {
			t.Fatal("arbitrary private route accepted", endpoint)
		}
	}
}
