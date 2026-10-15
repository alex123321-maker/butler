package config

import "testing"

func TestBrokerNetworkCapIsOperatorOnlyAndValidated(t *testing.T) {
	cfg, _, err := loadToolBroker(envMap(map[string]string{"BUTLER_NETWORK_ALLOWLISTS_JSON": `{"http":[],"browser":["*.example.com"]}`}))
	if err != nil {
		t.Fatal(err)
	}
	if cap, ok := cfg.NetworkAllowlists["http"]; !ok || len(cap) != 0 {
		t.Fatal("explicit empty cap lost")
	}
	for _, raw := range []string{`null`, `{"http":null}`, `{"unknown":[]}`, `{"http":["https://example.com"]}`, `{"http":["*"]}`} {
		if _, _, err := loadToolBroker(envMap(map[string]string{"BUTLER_NETWORK_ALLOWLISTS_JSON": raw})); err == nil {
			t.Fatal("invalid policy accepted", raw)
		}
	}
	if _, ok := managedSettingSpecByKey("BUTLER_NETWORK_ALLOWLISTS_JSON"); ok {
		t.Fatal("operator cap became mutable through settings")
	}
}

func TestSettingsHelpKeepsExistingCatalog(t *testing.T) {
	service := &SettingsService{env: envMap(map[string]string{}), hot: NewHotConfig(Snapshot{})}
	states, err := service.resolveStates(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Key == "BUTLER_LOG_LEVEL" {
			if state.Description == "" || state.ValueHint == "" {
				t.Fatal("missing help")
			}
			return
		}
	}
	t.Fatal("log setting missing")
}
