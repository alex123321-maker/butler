package config

import "testing"

func TestGuardedTabRuntimeRequiresOwningComposition(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_GUARDED_TAB_RUNTIME_ENABLED", Value: "true"}})
	if err != nil || c.GuardedTabRuntimeEnabled {
		t.Fatal("database enabled guarded runtime", err)
	}
	env["BUTLER_GUARDED_TAB_RUNTIME_ENABLED"] = "true"
	if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
		t.Fatal("runtime catalog enabled without owning requests")
	}
	delete(env, "BUTLER_GUARDED_TAB_RUNTIME_ENABLED")
	b, _, err := loadToolBroker(envMap(env))
	if err != nil || b.GuardedTabRuntimeEnabled {
		t.Fatal("runtime unexpectedly enabled", err)
	}
}

func TestGuardedTabOwnerTopologyRequiresOperatorAndPrivateDestination(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	settings := []Setting{{Key: "BUTLER_GUARDED_TAB_OWNER_ENABLED", Value: "true"}, {Key: "BUTLER_GUARDED_TAB_METADATA_URL", Value: "https://attacker.invalid/metadata"}}
	c, _, err := loadOrchestratorLayered(envMap(env), settings)
	if err != nil || c.GuardedTabOwnerEnabled || c.GuardedTabMetadataURL != "" {
		t.Fatal("database enabled owner topology", err)
	}
	env["BUTLER_GUARDED_TAB_OWNER_ENABLED"] = "true"
	if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
		t.Fatal("incomplete owner setup accepted")
	}
	env["BUTLER_GUARDED_TAB_HOST"] = "fixed-host"
	env["BUTLER_GUARDED_TAB_METADATA_URL"] = "http://127.0.0.1:19091/metadata"
	env["BUTLER_GUARDED_TAB_TOKEN_FILE"] = "/private/metadata-token"
	env["BUTLER_OWNER_CONTROL_TOKEN_FILE"] = "/private/owner-token"
	env["BUTLER_OWNER_CONTROL_OWNER_ID"] = "owner"
	for _, loader := range []func(envGetter) (OrchestratorConfig, Snapshot, error){loadOrchestrator, func(e envGetter) (OrchestratorConfig, Snapshot, error) { return loadOrchestratorLayered(e, nil) }} {
		c, _, err = loader(envMap(env))
		if err != nil || !c.GuardedTabOwnerEnabled || c.GuardedTabHost != "fixed-host" {
			t.Fatal("operator choice ignored", err)
		}
		for _, endpoint := range []string{"http://192.168.1.2/metadata", "http://localhost/metadata", "https://user:pass@host/metadata", "https://host/metadata?q=token", "https://host/metadata#token", "file:///metadata"} {
			env["BUTLER_GUARDED_TAB_METADATA_URL"] = endpoint
			if _, _, err = loader(envMap(env)); err == nil {
				t.Fatal("unsafe private destination accepted", endpoint)
			}
		}
		env["BUTLER_GUARDED_TAB_METADATA_URL"] = "http://127.0.0.1:19091/metadata"
	}
}

func TestGuardedTabRequestTopologyIsSeparateAndOperatorOnly(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_GUARDED_TAB_REQUEST_ENABLED", Value: "true"}})
	if err != nil || c.GuardedTabRequestEnabled {
		t.Fatal("database enabled worker request topology", err)
	}
	env["BUTLER_GUARDED_TAB_REQUEST_ENABLED"] = "true"
	if _, _, err = loadOrchestrator(envMap(env)); err == nil {
		t.Fatal("request enabled without owner composition")
	}
	env["BUTLER_GUARDED_TAB_OWNER_ENABLED"] = "true"
	env["BUTLER_GUARDED_TAB_HOST"] = "fixed-host"
	env["BUTLER_GUARDED_TAB_METADATA_URL"] = "http://127.0.0.1:19091/metadata"
	env["BUTLER_GUARDED_TAB_TOKEN_FILE"] = "/private/metadata-token"
	env["BUTLER_OWNER_CONTROL_TOKEN_FILE"] = "/private/owner-token"
	env["BUTLER_OWNER_CONTROL_OWNER_ID"] = "owner"
	if _, _, err = loadOrchestrator(envMap(env)); err == nil {
		t.Fatal("request enabled without Broker authentication")
	}
	env["BUTLER_ACTION_AUTHORITY_TOKEN_FILE"] = "/private/broker-token"
	c, _, err = loadOrchestrator(envMap(env))
	if err != nil || !c.GuardedTabRequestEnabled {
		t.Fatal("complete operator composition denied", err)
	}
	for _, endpoint := range []string{"http://192.0.2.1/internal/guarded-tab-request", "https://host/private-other", "https://host/internal/guarded-tab-request?token=x"} {
		env["BUTLER_GUARDED_TAB_REQUEST_CONTROL_URL"] = endpoint
		if _, _, err = loadToolBroker(envMap(env)); err == nil {
			t.Fatal("unsafe private Broker request destination accepted")
		}
	}
	env["BUTLER_GUARDED_TAB_REQUEST_CONTROL_URL"] = "http://127.0.0.1:8080/internal/guarded-tab-request"
	b, _, err := loadToolBroker(envMap(env))
	if err != nil || !b.GuardedTabRequestEnabled {
		t.Fatal("private Broker request config denied", err)
	}
}
