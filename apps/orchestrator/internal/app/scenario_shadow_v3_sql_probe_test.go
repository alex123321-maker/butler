package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
)

// Exercises the exact read-only SQL used after the native process, with three
// synthetic rows: known zero, known failed spend, and unavailable failed spend.
func TestScenarioShadowV3SQLExportProbe(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_SHADOW_V3_SQL_PROBE") != "1" {
		t.Skip("explicit zero-inference PostgreSQL export probe only")
	}
	type row struct {
		Ordinal       int       `json:"ordinal"`
		Phase         string    `json:"phase"`
		Case          string    `json:"case_id"`
		Role          string    `json:"role"`
		Context       string    `json:"context_id"`
		ParentContext *string   `json:"parent_context_id"`
		InputHash     string    `json:"input_hash"`
		Created       time.Time `json:"created_at"`
		Outcome       string    `json:"outcome"`
		Input         *int64    `json:"input_tokens"`
		Output        *int64    `json:"output_tokens"`
		Total         *int64    `json:"total_tokens"`
		OutputHash    *string   `json:"output_hash"`
	}
	var witness struct {
		Version string `json:"version"`
		Window  struct {
			Attempts int       `json:"attempts"`
			Baseline string    `json:"baseline_hash"`
			Policy   string    `json:"policy_hash"`
			Expiry   time.Time `json:"expires_at"`
		} `json:"window"`
		Attempts []row `json:"attempts"`
	}
	raw, err := acceptance.ReadPrivateFile("/pg-witness.json", 65536)
	if err != nil || acceptance.DecodeStrict(raw, &witness, "version", "window", "attempts") != nil {
		t.Fatal("exact PostgreSQL formatter output denied")
	}
	if witness.Version != "butler-cloud-v3-pg-witness-v1" || witness.Window.Attempts != 251 || witness.Window.Baseline != strings.Repeat("0", 64) || witness.Window.Policy != strings.Repeat("1", 64) || !witness.Window.Expiry.After(time.Now()) || len(witness.Attempts) != 3 {
		t.Fatal("synthetic read-only window counter/hash/expiry drift")
	}
	for i, a := range witness.Attempts {
		id := []string{"249", "250", "251"}[i]
		if a.Ordinal != 249+i || a.Phase != "quality" || a.Role != "single" || a.Case != "probe-case-"+id || a.Context != "probe-context-"+id || a.ParentContext != nil || a.InputHash != strings.Repeat("2", 64) || a.Created.IsZero() {
			t.Fatal("immutable metadata/order formatter drift")
		}
	}
	a, b, c := witness.Attempts[0], witness.Attempts[1], witness.Attempts[2]
	if a.Outcome != "completed" || a.Input == nil || a.Output == nil || a.Total == nil || *a.Input != 0 || *a.Output != 0 || *a.Total != 0 || a.OutputHash == nil || *a.OutputHash != strings.Repeat("3", 64) ||
		b.Outcome != "transport_error" || b.Input == nil || b.Output == nil || b.Total == nil || *b.Input != 10 || *b.Output != 2 || *b.Total != 15 || b.OutputHash != nil ||
		c.Outcome != "cancelled" || c.Input != nil || c.Output != nil || c.Total != nil || c.OutputHash != nil {
		t.Fatal("known zero, known failed cost or nullable unknown cost changed by SQL export")
	}
	// Ensure no field was silently ignored by the strict nested JSON decoder.
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != 3 {
		t.Fatal("SQL witness scope changed")
	}
}
