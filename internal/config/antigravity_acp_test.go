package config

import "testing"

func TestACPProcessConfigurationIsOperatorOnly(test *testing.T) {
	values := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/butler", "BUTLER_REDIS_URL": "redis://localhost/0", "BUTLER_MODEL_PROVIDER": "antigravity-acp", "BUTLER_ANTIGRAVITY_ACP_MODEL": "gemini-selected", "BUTLER_ANTIGRAVITY_ACP_EXECUTABLE": "/opt/agy", "BUTLER_ANTIGRAVITY_ACP_SHA256": "pinned", "BUTLER_ANTIGRAVITY_ACP_PROFILE_DIR": "/private/profile"}
	values["BUTLER_ANTIGRAVITY_ACP_SOCKET"] = "/private/acp.sock"
	values["BUTLER_MEMORY_MODEL_PROVIDER"] = "antigravity"
	env, _, err := loadOrchestrator(envMap(values))
	if err != nil {
		test.Fatal(err)
	}
	settings := []Setting{{Key: "BUTLER_ANTIGRAVITY_ACP_EXECUTABLE", Value: "/untrusted"}, {Key: "BUTLER_ANTIGRAVITY_ACP_MODEL", Value: "different"}, {Key: "BUTLER_ANTIGRAVITY_ACP_ENABLED", Value: "true"}}
	settings = append(settings, Setting{Key: "BUTLER_ANTIGRAVITY_ACP_SOCKET", Value: "/untrusted.sock"})
	settings = append(settings, Setting{Key: "BUTLER_MEMORY_MODEL_PROVIDER", Value: "openai"})
	layered, _, err := loadOrchestratorLayered(envMap(values), settings)
	if err != nil {
		test.Fatal(err)
	}
	if env.AntigravityACPEnabled || layered.AntigravityACPEnabled || env.AntigravityACPExecutable != "/opt/agy" || layered.AntigravityACPExecutable != env.AntigravityACPExecutable || layered.AntigravityACPModel != env.AntigravityACPModel || layered.ModelProvider != "antigravity-acp" {
		test.Fatal("operator ACP settings overridden")
	}
	if env.AntigravityACPSocket != "/private/acp.sock" || layered.AntigravityACPSocket != env.AntigravityACPSocket {
		test.Fatal("private IPC endpoint overridden through managed settings")
	}
	if env.MemoryModelProvider != "antigravity" || layered.MemoryModelProvider != env.MemoryModelProvider {
		test.Fatal("memory backend overridden through managed settings")
	}
	values["BUTLER_MEMORY_MODEL_PROVIDER"] = "antigravity-acp"
	selected, _, err := loadOrchestrator(envMap(values))
	if err != nil || selected.MemoryModelProvider != "antigravity-acp" {
		test.Fatal("explicit independent ACP memory backend rejected", err)
	}
	for _, key := range []string{"BUTLER_ANTIGRAVITY_ACP_ENABLED", "BUTLER_ANTIGRAVITY_ACP_MODEL", "BUTLER_ANTIGRAVITY_ACP_EXECUTABLE", "BUTLER_ANTIGRAVITY_ACP_SHA256", "BUTLER_ANTIGRAVITY_ACP_PROFILE_DIR"} {
		if _, ok := managedSettingSpecByKey(key); ok {
			test.Fatalf("%s exposed through settings", key)
		}
	}
	providerSpec, _ := managedSettingSpecByKey("BUTLER_MODEL_PROVIDER")
	for _, provider := range providerSpec.Spec.allowedValues {
		if provider == "antigravity-acp" {
			test.Fatal("unqualified provider advertised in managed settings")
		}
	}
}
