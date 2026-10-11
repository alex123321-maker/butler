package config

import "testing"

func TestConverterDelegationCompositionIsOperatorOnly(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	settings := []Setting{{Key: "BUTLER_CONVERTER_DELEGATION_ENABLED", Value: "true"}, {Key: "BUTLER_CONVERTER_PROFILE_JSON", Value: `{"untrusted":"database"}`}}
	c, _, err := loadOrchestratorLayered(envMap(env), settings)
	if err != nil || c.ConverterDelegationEnabled || c.ConverterProfileJSON != "" {
		t.Fatal("database enabled or repinned converter delegation", err)
	}
	env["BUTLER_CONVERTER_DELEGATION_ENABLED"] = "true"
	env["BUTLER_CONVERTER_PROFILE_JSON"] = `{"operator":"profile"}`
	for _, load := range []func(envGetter) (OrchestratorConfig, Snapshot, error){loadOrchestrator, func(get envGetter) (OrchestratorConfig, Snapshot, error) {
		return loadOrchestratorLayered(get, settings)
	}} {
		c, _, err = load(envMap(env))
		if err != nil || !c.ConverterDelegationEnabled || c.ConverterProfileJSON != env["BUTLER_CONVERTER_PROFILE_JSON"] {
			t.Fatal("operator composition was ignored", err)
		}
	}
	b, _, err := loadToolBroker(envMap(map[string]string{}))
	if err != nil || b.ConverterDelegationEnabled || b.ConverterDelegationControlURL != "" {
		t.Fatal("developer control enabled by default", err)
	}
	for _, endpoint := range []string{"http://core/internal/other", "http://user:password@core/internal/converter-delegation", "http://core/internal/converter-delegation?q=data", "http://core/internal/converter-delegation#x", "http://core/internal/%63onverter-delegation", "file:///internal/converter-delegation", "http://core/internal/converter-delegation/", " http://core/internal/converter-delegation "} {
		if _, _, err = loadToolBroker(envMap(map[string]string{"BUTLER_CONVERTER_DELEGATION_CONTROL_URL": endpoint})); err == nil {
			t.Fatal("unsafe operator control endpoint accepted", endpoint)
		}
	}
	b, _, err = loadToolBroker(envMap(map[string]string{"BUTLER_CONVERTER_DELEGATION_ENABLED": "true", "BUTLER_CONVERTER_DELEGATION_CONTROL_URL": "http://core:8080/internal/converter-delegation"}))
	if err != nil || !b.ConverterDelegationEnabled || b.ConverterDelegationControlURL != "http://core:8080/internal/converter-delegation" {
		t.Fatal("fixed operator control ignored", err)
	}
}
