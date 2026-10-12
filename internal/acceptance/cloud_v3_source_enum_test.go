package acceptance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func v3SchemaIDValues(t *testing.T, raw []byte, kind string) []string {
	t.Helper()
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		t.Fatal("schema JSON")
	}
	properties := schema["properties"].(map[string]any)
	items := properties[map[string]string{"retrieval": "refs", "extraction": "facts"}[kind]].(map[string]any)["items"].(map[string]any)
	if kind == "extraction" {
		items = items["properties"].(map[string]any)["source_id"].(map[string]any)
	}
	result := []string{}
	for _, value := range items["enum"].([]any) {
		result = append(result, value.(string))
	}
	return result
}

func TestCloudV3NativeSourceEnumContainsEveryOriginalNeverGoldOrBody(t *testing.T) {
	documents := map[string]string{"srcZ-original": "PRIVATE_DATA_DISTRACTOR", "srcA-original": "PRIVATE_DATA_CORRECT"}
	for _, kind := range []string{"retrieval", "extraction"} {
		raw, err := CloudV3ResponseSchema(kind, documents)
		if err != nil || len(raw) > 16384 || bytes.Contains(raw, []byte("PRIVATE_DATA")) || bytes.Contains(raw, []byte("$ref")) ||
			!reflect.DeepEqual(v3SchemaIDValues(t, raw, kind), []string{"srcA-original", "srcZ-original"}) {
			t.Fatal("schema selected a witness, exposed body or changed original namespace", kind, err)
		}
		second, _ := CloudV3ResponseSchema(kind, map[string]string{"srcA-original": "different body", "srcZ-original": "different body"})
		if !bytes.Equal(raw, second) {
			t.Fatal("native namespace schema depended on source prose")
		}
	}
	for _, kind := range []string{"classification", "commands"} {
		a, _ := CloudV3ResponseSchema(kind, documents)
		b, _ := CloudV3ResponseSchema(kind, nil)
		if !bytes.Equal(a, b) || bytes.Contains(a, []byte("srcA-original")) {
			t.Fatal("non-source schema leaked case-selected metadata")
		}
	}
	for _, id := range []string{"", strings.Repeat("a", 129), string([]byte{0xff})} {
		if _, err := CloudV3ResponseSchema("retrieval", map[string]string{id: "body"}); err == nil {
			t.Fatal("invalid original ID accepted")
		}
	}
	for _, kind := range []string{"retrieval", "extraction"} {
		raw, err := CloudV3ResponseSchema(kind, nil)
		if err != nil || !bytes.Contains(raw, []byte(`"maxItems":0`)) || bytes.Contains(raw, []byte(`"enum":[]`)) {
			t.Fatal("empty source map allows nonempty answer or invalid enum")
		}
	}
}

func TestCloudV3SourceNamespaceAndQuotationAreNeverRepaired(t *testing.T) {
	e := Example{Kind: "retrieval", Documents: map[string]string{"srcA": "original fact", "srcB": "distractor"}, Golden: json.RawMessage(`{"refs":["srcA"]}`)}
	for _, raw := range []string{`{"refs":["doc:srcA"]}`, `{"refs":["source:srcA"]}`, `{"refs":["srca"]}`, `{"refs":["srcA","srcA"]}`} {
		check := VerifyCloudV3Response(e, []byte(raw))
		if check.Exact || !check.SourceFabrication {
			t.Fatal("foreign namespace/duplicate repaired")
		}
	}
	check := VerifyCloudV3Response(e, []byte(`{"refs":["srcB"]}`))
	if check.Exact || check.SourceFabrication {
		t.Fatal("all-ID enum preselected correct answer or mislabeled valid distractor")
	}
	e.Kind = "extraction"
	e.Golden = json.RawMessage(`{"facts":[{"key":"field","value":"original fact","source_id":"srcA","quote":"original fact"}]}`)
	for _, raw := range []string{
		`{"facts":[{"key":"field","value":"original fact","source_id":"doc:srcA","quote":"original fact"}]}`,
		`{"facts":[{"key":"field","value":"paraphrase","source_id":"srcA","quote":"original fact"}]}`,
		`{"facts":[{"key":"field","value":"original fact","source_id":"srcB","quote":"original fact"}]}`,
	} {
		check := VerifyCloudV3Response(e, []byte(raw))
		if check.Exact || !check.SourceFabrication {
			t.Fatal("ID/quote/value witness repaired")
		}
	}
}

func TestCloudV3EverySourceEnumAndChildDataByteAreInActualInputDigest(t *testing.T) {
	input := CloudInputCase{ID: "case:opaque", Kind: "retrieval", Input: "Which original?", Documents: map[string]string{"srcA": "first", "srcZ": "second"}, OriginalInputSHA256: strings.Repeat("a", 64)}
	x, err := MakeCloudV3ModelInput(input, CloudRoleSingle, nil)
	if err != nil || strings.Contains(x.SystemInstructions, "doc:<id>") || !strings.Contains(x.SystemInstructions, "точные ключи объекта documents") ||
		!reflect.DeepEqual(v3SchemaIDValues(t, x.Schema, "retrieval"), []string{"srcA", "srcZ"}) {
		t.Fatal("namespace instruction/model schema mismatch", err)
	}
	input.Documents = map[string]string{"srcZ": "second", "srcA": "first"}
	reordered, _ := MakeCloudV3ModelInput(input, CloudRoleSingle, nil)
	if CloudV3ModelInputDigest(x) != CloudV3ModelInputDigest(reordered) {
		t.Fatal("map order changed digest")
	}
	input.Documents = map[string]string{"srcA": "first"}
	less, _ := MakeCloudV3ModelInput(input, CloudRoleSingle, nil)
	if CloudV3ModelInputDigest(x) == CloudV3ModelInputDigest(less) || bytes.Equal(x.Schema, less.Schema) {
		t.Fatal("missing original enum was not pinned")
	}
	tampered := x
	tampered.Schema = less.Schema
	if CloudV3ModelInputDigest(x) == CloudV3ModelInputDigest(tampered) {
		t.Fatal("full native schema not included in input hash")
	}
	input.Documents = map[string]string{"srcA": "first", "srcZ": "second"}
	synthesis, err := MakeCloudV3ModelInput(input, CloudRoleSynthesis, []string{`{"refs":["srcA"]}`, `{"refs":["srcZ"]}`})
	if err != nil || !strings.Contains(synthesis.SystemInstructions, cloudSynthesisGuard) || !bytes.Equal(synthesis.Schema, x.Schema) {
		t.Fatal("child DATA affected original namespace", err)
	}
	changed, _ := MakeCloudV3ModelInput(input, CloudRoleSynthesis, []string{`{"refs":["srcA"]}`, `{"refs":[]}`})
	if CloudV3ModelInputDigest(changed) == CloudV3ModelInputDigest(synthesis) {
		t.Fatal("child answer change lost from digest")
	}
}
