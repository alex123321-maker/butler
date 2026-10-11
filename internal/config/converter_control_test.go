package config

import "testing"

func TestGeneratedConverterOperatorOnlyFixedRoutes(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0", "BUTLER_CONVERTER_CONTROL_URL": "http://tool-broker:8081/internal/converters"}
	settings := []Setting{{Key: "BUTLER_CONVERTER_CONTROL_URL", Value: "http://collect.example/internal/converters"}}
	c, _, e := loadOrchestratorLayered(envMap(env), settings)
	if e != nil || c.ConverterControlURL != env["BUTLER_CONVERTER_CONTROL_URL"] {
		t.Fatal("operator endpoint lost", e)
	}
	delete(env, "BUTLER_CONVERTER_CONTROL_URL")
	c, _, e = loadOrchestratorLayered(envMap(env), settings)
	if e != nil || c.ConverterControlURL != "" {
		t.Fatal("stored setting redirected service authentication", e)
	}
	for _, v := range []string{"http://broker/internal/other", "http://u:p@broker/internal/converters", "http://broker/internal/converters?q=secret", "http://broker/internal/converters#x", "http://broker/internal/%63onverters", "file:///internal/converters", "http://broker/internal/converters/"} {
		env["BUTLER_CONVERTER_CONTROL_URL"] = v
		if _, _, e = loadOrchestratorLayered(envMap(env), nil); e == nil {
			t.Fatal("unsafe private endpoint accepted", v)
		}
	}
	base := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test"}
	b, _, e := loadToolBroker(envMap(base))
	if e != nil || b.ConvertersEnabled {
		t.Fatal("generated runtime enabled by default", e)
	}
	for _, v := range []string{"http://collect.example", "dns:///converter:9090", "unix:///tmp/runtime.sock"} {
		base["BUTLER_CONVERTER_RUNTIME_TARGET"] = v
		if _, _, e = loadToolBroker(envMap(base)); e == nil {
			t.Fatal("runtime target selectable", v)
		}
	}
}
