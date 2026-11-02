package registry

import (
	"path/filepath"
	"testing"
)

func TestFrozenObservationSchemaAcceptsRuntimeObservation(t *testing.T) {
	reg, err := Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "browser:9090")
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := reg.Get("browser.freeze_snapshot")
	if !ok {
		t.Fatal("frozen observation missing")
	}
	for _, sample := range []struct {
		raw   string
		valid bool
	}{
		{`{"ok":true,"browser_session_id":"context","observation_version":2,"network_state":"frozen","text":"Current document"}`, true},
		{`{"ok":true,"browser_session_id":"context","observation_version":2,"network_state":"read_only"}`, false},
		{`{"ok":false,"browser_session_id":"context","observation_version":2,"network_state":"frozen"}`, false},
		{`{"ok":true,"browser_session_id":"context","observation_version":2}`, false},
	} {
		if err := ValidateArgs(contract.OutputSchemaJson, sample.raw); (err == nil) != sample.valid {
			t.Fatalf("runtime output validation disagrees: %v", err)
		}
	}
	if err := ValidateArgs(contract.InputSchemaJson, `{"browser_session_id":"context","url":"https://example.com"}`); err == nil {
		t.Fatal("frozen read accepted navigation args")
	}
}
