package config

import "testing"

func TestServerBrowserControlSurvivesActualStartupConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func(envGetter) (OrchestratorConfig, Snapshot, error)
	}{
		{"environment", loadOrchestrator},
		{"layered", func(get envGetter) (OrchestratorConfig, Snapshot, error) {
			return loadOrchestratorLayered(get, []Setting{{Key: "BUTLER_BROWSER_CONTROL_URL", Value: "https://untrusted.invalid/internal/server-browser", Component: "orchestrator"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost/butler", "BUTLER_REDIS_URL": "redis://localhost/0", "BUTLER_ACTION_AUTHORITY_TOKEN_FILE": "/private/operator-proof"}
			cfg, _, err := tc.load(envMap(env))
			if err != nil || cfg.BrowserControlURL != "" {
				t.Fatal("stored settings enabled a private service route", err)
			}
			env["BUTLER_BROWSER_CONTROL_URL"] = "  http://tool-broker:8080/internal/server-browser  "
			cfg, _, err = tc.load(envMap(env))
			if err != nil || cfg.BrowserControlURL != "http://tool-broker:8080/internal/server-browser" {
				t.Fatal("operator browser endpoint was lost at startup", err)
			}
			for _, endpoint := range []string{"http://user:secret@broker/internal/server-browser", "http://broker/wrong-path", "http://broker/internal/server-browser?target=other", "http://broker/internal/server-browser#fragment"} {
				env["BUTLER_BROWSER_CONTROL_URL"] = endpoint
				if _, _, err := tc.load(envMap(env)); err == nil {
					t.Fatal("invalid authenticated service destination accepted")
				}
			}
			env["BUTLER_BROWSER_CONTROL_URL"] = "http://tool-broker:8080/internal/server-browser"
			delete(env, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE")
			if _, _, err := tc.load(envMap(env)); err == nil {
				t.Fatal("private browser endpoint enabled without Broker service proof")
			}
		})
	}
}
