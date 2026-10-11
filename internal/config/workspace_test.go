package config

import "testing"

func TestWorkspaceTopologyIsOperatorOnlyAndPrivate(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	c, _, err := loadOrchestratorLayered(envMap(env), []Setting{{Key: "BUTLER_WORKSPACE_ENABLED", Value: "true"}})
	if err != nil || c.WorkspaceEnabled {
		t.Fatal("stored setting enabled workspace topology", err)
	}
	env["BUTLER_WORKSPACE_ENABLED"] = "true"
	if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
		t.Fatal("missing private action authority")
	}
	env["BUTLER_ACTION_AUTHORITY_TOKEN_FILE"] = "/private/fixture-token"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.WorkspaceEnabled {
		t.Fatal("operator choice unavailable", err)
	}
	for _, target := range []string{"localhost:9096", "192.0.2.1:9096", ":9096", "dns:///127.0.0.1:9096", "unix:///tmp/other.sock"} {
		env["BUTLER_WORKSPACE_RUNTIME_TARGET"] = target
		if _, _, err = loadToolBroker(envMap(env)); err == nil {
			t.Fatal("unsafe workspace target", target)
		}
	}
	for _, target := range []string{"127.0.0.1:9096", "[::1]:9096", "unix:///run/butler-workspace/runtime.sock"} {
		env["BUTLER_WORKSPACE_RUNTIME_TARGET"] = target
		if _, _, err = loadToolBroker(envMap(env)); err != nil {
			t.Fatal("fixed private target", err)
		}
	}
	runtimeEnv := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_WORKSPACE_PROFILE_ID": "fixture", "BUTLER_WORKSPACE_ROOT": "/private/workspaces", "BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE": "/private/fixture-token", "BUTLER_MEMORY_DELETION_MANIFEST_FILE": "/private/deletions.log"}
	if r, _, err := loadToolWorkspace(envMap(runtimeEnv)); err != nil || r.Shared.GRPCAddr != "127.0.0.1:9096" {
		t.Fatal("default runtime topology", err)
	}
	runtimeEnv["BUTLER_HTTP_ADDR"] = ":8086"
	if _, _, err = loadToolWorkspace(envMap(runtimeEnv)); err == nil {
		t.Fatal("public runtime listener admitted")
	}
}
