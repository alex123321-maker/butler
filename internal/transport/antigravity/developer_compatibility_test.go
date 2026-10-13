package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
)

func TestDeveloperArgsSchemaDecoderCompatibility(t *testing.T) {
	ref := `{"source_id":"need","revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("a", 64) + `"}`
	spec := strings.Replace(ref, `"source_id":"need"`, `"source_id":"contract"`, 1)
	base := `{"need_source":` + ref + `,"contract_source":` + spec + `,"budget":{"model_turns":2,"tokens":4096,"cost_micros":0,"external_calls":4,"children":0,"active_millis":1320000,"egress_bytes":4096,"provider_input_bytes":4096},"deadline":"2026-10-14T00:00:00Z","dependencies":[]}`
	contract := generatedtools.DeveloperAdmissionContract()
	if validateSchema(contract.InputSchemaJson) != nil {
		t.Fatal("declared contract not supported")
	}
	for _, tc := range []struct {
		name, raw string
		decode    bool
	}{
		{"plain_integer", base, true},
		{"decimal_integer", strings.Replace(base, `"model_turns":2`, `"model_turns":2.0`, 1), true},
		{"exponent_integer", strings.Replace(base, `"model_turns":2`, `"model_turns":2e0`, 1), true},
		{"timezone_required_beyond_schema_string", strings.Replace(base, `2026-10-14T00:00:00Z`, `2026-10-14`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateArguments(contract.InputSchemaJson, json.RawMessage(tc.raw)); err != nil {
				t.Fatal("schema rejected exact integer or string", err)
			}
			_, err := generatedtools.DecodeDeveloperArgs(tc.raw)
			if (err == nil) != tc.decode {
				t.Fatal("declared valid integer differs from typed decoder", err)
			}
		})
	}
}
