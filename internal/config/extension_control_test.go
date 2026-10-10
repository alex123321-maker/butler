package config

import "testing"

func TestLayeredExtensionControlUsesOperatorEndpoint(t *testing.T) {
	environment := map[string]string{
		"BUTLER_POSTGRES_URL":          "postgres://localhost:5432/butler",
		"BUTLER_REDIS_URL":             "redis://localhost:6379/0",
		"BUTLER_EXTENSION_CONTROL_URL": "http://tool-broker:8080/internal/extensions",
	}
	settings := []Setting{{Key: "BUTLER_EXTENSION_CONTROL_URL", Value: "http://untrusted.example/collect", Component: "orchestrator"}}
	cfg, _, err := loadOrchestratorLayered(envMap(environment), settings)
	if err != nil || cfg.ExtensionControlURL != environment["BUTLER_EXTENSION_CONTROL_URL"] {
		t.Fatal("operator endpoint lost during layered startup")
	}
	delete(environment, "BUTLER_EXTENSION_CONTROL_URL")
	cfg, _, err = loadOrchestratorLayered(envMap(environment), settings)
	if err != nil || cfg.ExtensionControlURL != "" {
		t.Fatal("stored settings selected a service-auth destination")
	}
	environment["BUTLER_EXTENSION_CONTROL_URL"] = "://invalid"
	if _, _, err = loadOrchestratorLayered(envMap(environment), nil); err == nil {
		t.Fatal("invalid operator endpoint accepted")
	}
}
