package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloudWitnessDiagnosticsSeparatesAnchoredContractMismatch(t *testing.T) {
	e := Example{Kind: "extraction", Documents: map[string]string{"original": "PRIVATE_ORIGINAL exact witness"}, Golden: json.RawMessage(`{"facts":[]}`)}
	for _, tc := range []struct {
		name, source, key, quote, value string
		want                            CloudWitnessCounts
	}{
		{"anchored_value_only", "original", "key", "exact witness", "different value", CloudWitnessCounts{Cases: 1, ValueQuoteMismatch: 1, AnchoredValueQuoteMismatch: 1, OnlyAnchoredValueQuoteMismatch: 1}},
		{"invalid_source", "foreign", "key", "exact witness", "exact witness", CloudWitnessCounts{Cases: 1, InvalidSource: 1, OtherWitnessFailure: 1}},
		{"absent_quote", "original", "key", "absent", "absent", CloudWitnessCounts{Cases: 1, QuoteAbsent: 1, OtherWitnessFailure: 1}},
		{"empty_quote", "original", "key", "", "", CloudWitnessCounts{Cases: 1, EmptyQuote: 1, OtherWitnessFailure: 1}},
		{"empty_key", "original", "", "exact witness", "exact witness", CloudWitnessCounts{Cases: 1, EmptyKey: 1, OtherWitnessFailure: 1}},
		{"exact", "original", "key", "exact witness", "exact witness", CloudWitnessCounts{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"facts": []any{map[string]string{"key": tc.key, "value": tc.value, "source_id": tc.source, "quote": tc.quote}}})
			got := cloudWitnessReasons(e, raw)
			if got != tc.want {
				t.Fatal("witness category mismatch", got)
			}
			encoded, _ := json.Marshal(got)
			for _, private := range []string{"PRIVATE_ORIGINAL", "different value", "exact witness", "foreign"} {
				if strings.Contains(string(encoded), private) {
					t.Fatal("content escaped diagnostics")
				}
			}
		})
	}
	r := Example{Kind: "retrieval", Documents: map[string]string{"original": "PRIVATE_ORIGINAL"}, Golden: json.RawMessage(`{"refs":[]}`)}
	if got := cloudWitnessReasons(r, []byte(`{"refs":["original","original","foreign"]}`)); got != (CloudWitnessCounts{Cases: 1, InvalidSource: 1, DuplicateRef: 1, OtherWitnessFailure: 1}) {
		t.Fatal("retrieval diagnostics", got)
	}
}

func TestCloudWindowWitnessDiagnosticsPreserveFrozenVerdict(t *testing.T) {
	c, a, key, e, expected := cloudWindowFixture(t)
	var selected Example
	var index int
	for _, original := range c.Examples {
		if original.Kind == "extraction" && !original.NoModel {
			selected = original
			break
		}
	}
	for i := range e.Attempts {
		if e.Attempts[i].CaseID == cloudCaseID(selected.ID) {
			index = i
			break
		}
	}
	var out struct {
		Facts []map[string]string `json:"facts"`
	}
	if json.Unmarshal(selected.Golden, &out) != nil || len(out.Facts) == 0 {
		t.Fatal("golden fixture")
	}
	out.Facts[0]["value"] = "PRIVATE_DIFFERENT_VALUE"
	e.Attempts[index].Output, _ = json.Marshal(out)
	r, err := AuditCloudWindow(c, a, key, windowFixtureBytes(t, e), expected)
	if err != nil || r.Status != "NO_GO" || r.WitnessDiagnostics.Total != (CloudWitnessCounts{Cases: 1, ValueQuoteMismatch: 1, AnchoredValueQuoteMismatch: 1, OnlyAnchoredValueQuoteMismatch: 1}) || r.Quality.Splits[selected.Split].SourceFabrications != 1 {
		t.Fatal("diagnostics changed scoring", err, r.Status, r.WitnessDiagnostics.Total)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "PRIVATE_DIFFERENT_VALUE") {
		t.Fatal("private value in safe audit")
	}
	if CloudPolicyDigest() != "20313d5ec68ef865f51fa0c49a10f8ae9443a66f1e4e2a51c309a1d67073fafb" || CloudSchemaDigest() != "a0009cc050b4f46f5efdc84efec67bcbc07dd3d18add61c34e28ccb330e422f2" {
		t.Fatal("frozen thresholds changed")
	}
}
