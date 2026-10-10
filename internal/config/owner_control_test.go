package config

import "testing"

func TestOwnerControlCannotBeAssignedByDatabaseSettings(t *testing.T) {
	settings := []Setting{{Key: "BUTLER_OWNER_CONTROL_TOKEN", Value: "db-attacker-token-0000000000000000", Component: "orchestrator"}, {Key: "BUTLER_OWNER_CONTROL_OWNER_ID", Value: "attacker", Component: "orchestrator"}}
	environment := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/butler", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	cfg, _, err := loadOrchestratorLayered(envMap(environment), settings)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OwnerControlToken != "" || cfg.OwnerControlOwnerID != "" {
		t.Fatal("database assigned trusted owner authority")
	}
	environment["BUTLER_OWNER_CONTROL_TOKEN_FILE"] = "/private/owner.token"
	environment["BUTLER_OWNER_CONTROL_OWNER_ID"] = "owner"
	cfg, _, err = loadOrchestratorLayered(envMap(environment), settings)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OwnerControlOwnerID != "owner" || cfg.OwnerControlTokenFile != "/private/owner.token" || cfg.OwnerControlToken != "" {
		t.Fatal("database overrode operator owner authority")
	}
}
