package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
)

type v5NeverAuthenticate struct{ calls *int }

func (s v5NeverAuthenticate) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	*s.calls++
	return providerauth.AntigravityAuth{}, errors.New("constructor must not authenticate")
}

func TestCloudV5EveryEligibleSchemaAcceptedByActualNativeConstructorWithoutAuth(t *testing.T) {
	raw, err := os.ReadFile("../../" + CloudV5CorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCloudV5Corpus(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := ExportCloudV5Inputs(c, "")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, input := range inputs.Cases {
		model, err := MakeCloudV5ModelInput(input, CloudRoleSingle, nil)
		if err != nil {
			t.Fatal("model input", input.ID, err)
		}
		_, err = antigravity.NewProvider(antigravity.Config{Model: CloudRequestedModel, AuthSource: v5NeverAuthenticate{&calls}, MaximumOutputTokens: model.MaximumGeneratedTokens, GoogleResponseSchema: model.Schema})
		if err != nil {
			t.Fatal("actual native constructor rejected eligible schema", input.ID, err)
		}
	}
	if len(inputs.Cases) != 236 || len(inputs.BlockedIDs) != 4 || calls != 0 {
		t.Fatal("constructor inventory/auth boundary")
	}
}

func TestCloudV5NativeArrayCapAndEmptyOriginalWitnessCannotFabricateSource(t *testing.T) {
	for _, kind := range []string{"retrieval", "extraction"} {
		gold := json.RawMessage(`{"refs":[]}`)
		nonempty := json.RawMessage(`{"refs":["not_provided"]}`)
		if kind == "extraction" {
			gold = json.RawMessage(`{"facts":[]}`)
			nonempty = json.RawMessage(`{"facts":[{"key":"field","value":"x","source_id":"not_provided","quote":"x"}]}`)
		}
		e := Example{Kind: kind, Documents: map[string]string{}, Golden: gold}
		good := VerifyCloudV5Response(e, gold)
		bad := VerifyCloudV5Response(e, nonempty)
		if !good.Exact || good.SchemaError || good.SourceFabrication || !bad.SourceFabrication {
			t.Fatal("positive native syntax ceiling fabricated an empty-map original", kind)
		}
		schema, err := CloudV5ResponseSchema(kind, nil)
		if err != nil || bytes.Contains(schema, []byte("not_provided")) || !bytes.Contains(schema, []byte(`"maxItems":"32"`)) {
			t.Fatal("empty-map producer recipe", kind)
		}
	}
	fact := map[string]string{"key": "field", "value": "x", "source_id": "provided", "quote": "x"}
	facts := []map[string]string{}
	for i := 0; i < 33; i++ {
		facts = append(facts, fact)
	}
	raw, _ := json.Marshal(map[string]any{"facts": facts})
	e := Example{Kind: "extraction", Documents: map[string]string{"provided": "x"}, Golden: json.RawMessage(`{"facts":[]}`)}
	if !VerifyCloudV5Response(e, raw).SchemaError {
		t.Fatal("33-fact native-array contract accepted")
	}
}

func TestCloudSchemaDialectDiagnosticIndependentOriginalWitnessesAndOldLeakageScreen(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/acceptance/cloud-schema-dialect-diagnostic-20261013.json")
	if err != nil {
		t.Fatal(err)
	}
	var inputs struct {
		Version, Provenance, Purpose string
		Cases                        []struct {
			ID, Kind, Input string
			Documents       map[string]string
			Golden          json.RawMessage
		}
	}
	if json.Unmarshal(raw, &inputs) != nil || inputs.Version != "butler-cloud-schema-dialect-diagnostic-inputs-v1" || inputs.Provenance != "synthetic_public" || len(inputs.Cases) != 2 {
		t.Fatal("public diagnostic inventory")
	}
	texts := []freshV5Text{}
	for _, c := range inputs.Cases {
		check := VerifyCloudV4Response(Example{Kind: c.Kind, Documents: c.Documents, Golden: c.Golden}, c.Golden)
		if !check.Exact || check.SchemaError || check.SourceFabrication || check.UnsafeAuthority || c.Kind != "extraction" || len(c.Documents) != 2 {
			t.Fatal("independent diagnostic original witness", c.ID)
		}
		texts = append(texts, freshV5TextOf(c.ID, "", "", c.Input))
		for id, body := range c.Documents {
			if !freshV5SourceID.MatchString(id) {
				t.Fatal("diagnostic original ID")
			}
			texts = append(texts, freshV5TextOf(id, "", "", body))
		}
	}
	for _, name := range []string{"ru-scenarios-v1.json", "ru-scenarios-v2.json", "ru-scenarios-v3.json", "ru-cloud-fresh-v2.json", "ru-cloud-fresh-v3.json", "ru-cloud-fresh-v4.json", "cloud-structural-json-diagnostic-20261012.json"} {
		old, err := os.ReadFile("../../testdata/acceptance/" + name)
		var object any
		if err != nil || json.Unmarshal(old, &object) != nil {
			t.Fatal("old observed material missing")
		}
		var walk func(any, string)
		walk = func(v any, field string) {
			switch x := v.(type) {
			case map[string]any:
				for k, child := range x {
					if field == "documents" {
						walk(child, "document")
					} else {
						walk(child, k)
					}
				}
			case []any:
				for _, child := range x {
					walk(child, field)
				}
			case string:
				if field == "document" || field == "input" || field == "question" || field == "missing_question" {
					other := freshV5TextOf(name+":"+field, "", "", x)
					for _, text := range texts {
						if freshV5Near(text, other) {
							t.Fatal("new diagnostic copied observed scaffold", text.id, other.id)
						}
					}
				}
			}
		}
		walk(object, "")
	}
	t.Logf("diagnostic_raw_sha256=%s", hashWindowBytes(raw))
}
