package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/butler/butler/internal/skills"
)

// This is only the output interface, derived from the actual parser's Go types.
// It contains no procedure, example skill, note content, case or oracle. The
// existing strict parser and semantic lifecycle remain the only acceptance gate.
func experienceManifestWireSchema(t *testing.T) map[string]any {
	t.Helper()
	var wire func(reflect.Type) map[string]any
	wire = func(typ reflect.Type) map[string]any {
		switch typ.Kind() {
		case reflect.String:
			return map[string]any{"type": "string"}
		case reflect.Bool:
			return map[string]any{"type": "boolean"}
		case reflect.Int64:
			return map[string]any{"type": "integer"}
		case reflect.Slice:
			return map[string]any{"type": "array", "items": wire(typ.Elem())}
		case reflect.Struct:
			properties := map[string]any{}
			required := []string{}
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				parts := strings.Split(f.Tag.Get("json"), ",")
				if !f.IsExported() || parts[0] == "" || parts[0] == "-" {
					t.Fatal("unregistered manifest wire field")
				}
				properties[parts[0]] = wire(f.Type)
				if !strings.Contains(f.Tag.Get("json"), ",omitempty") {
					required = append(required, parts[0])
				}
			}
			return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		default:
			t.Fatalf("unsupported manifest wire type: %s", typ)
			return nil
		}
	}
	schema := wire(reflect.TypeFor[skills.Manifest]())
	properties := schema["properties"].(map[string]any)
	properties["format"].(map[string]any)["const"] = skills.Format
	for name, limits := range map[string][2]int{"when": {1, 8}, "inputs": {1, 16}, "not_applicable": {1, 16}, "dependencies": {1, 16}, "steps": {1, 24}, "tests": {3, 3}, "sources": {1, 8}, "lesson_sources": {0, 8}} {
		field := properties[name].(map[string]any)
		field["minItems"], field["maxItems"] = limits[0], limits[1]
	}
	return schema
}

func TestExperienceManifestWireContractContainsOnlyTypeMetadata(t *testing.T) {
	schema := experienceManifestWireSchema(t)
	raw, err := json.Marshal(schema)
	if err != nil || len(raw) > 8192 {
		t.Fatal("bounded manifest wire schema required", err)
	}
	properties := schema["properties"].(map[string]any)
	// In particular, arrays are not ambiguous objects and the nested references
	// expose the parser's real revision/hash names, not invented example values.
	for field, want := range map[string]reflect.Type{"dependencies": reflect.TypeFor[skills.Dependency](), "steps": reflect.TypeFor[skills.Step](), "tests": reflect.TypeFor[skills.Scenario](), "sources": reflect.TypeFor[skills.EvidenceRef]()} {
		array := properties[field].(map[string]any)
		item := array["items"].(map[string]any)
		actual := item["properties"].(map[string]any)
		if array["type"] != "array" || item["additionalProperties"] != false || len(actual) != want.NumField() {
			t.Fatal("nested wire contract lost parser fields", field)
		}
		for i := 0; i < want.NumField(); i++ {
			if actual[strings.Split(want.Field(i).Tag.Get("json"), ",")[0]] == nil {
				t.Fatal("parser wire name omitted", field)
			}
		}
	}
	for _, forbidden := range []string{"/project/", "credential", "verified_success", "prepare-patch", "success_check\":\""} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("manifest shape contains procedural content", forbidden)
		}
	}
}
