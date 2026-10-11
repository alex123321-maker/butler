package delegation

import (
	"encoding/json"
	"strings"
	"testing"
)

func numericFixture(t *testing.T, value int64) (SourceRef, string, string) {
	t.Helper()
	source := SourceRef{ID: "source:original", Revision: 2, AccessRevision: 3, Hash: strings.Repeat("a", 64)}
	body, err := json.Marshal(NumericObservation{source.ID, source.Revision, source.Hash, value, "2026-10-11", "source_only"})
	if err != nil {
		t.Fatal(err)
	}
	witness, _ := json.Marshal(struct {
		Value int64  `json:"observed_value"`
		Date  string `json:"observed_date"`
	}{value, "2026-10-11"})
	return source, string(body), string(witness)
}

func TestNumericObservationChecksOriginalInsteadOfAgreement(t *testing.T) {
	for _, value := range []int64{0, -1, 17, 9223372036854775807, -9223372036854775808} {
		source, raw, witness := numericFixture(t, value)
		if err := VerifyNumericObservation([]byte(raw), source, []byte(witness)); err != nil {
			t.Fatal("exact integer observation denied", value, err)
		}
	}
	source, raw, witness := numericFixture(t, 17)
	wrong := strings.Replace(raw, `"observed_value":17`, `"observed_value":23`, 1)
	for i := 0; i < 2; i++ {
		if VerifyNumericObservation([]byte(wrong), source, []byte(witness)) == nil {
			t.Fatal("agreeing wrong observations became original evidence")
		}
	}
	if VerifyNumericObservation([]byte(raw), source, []byte(`{"observed_value":17,"observed_date":"2026-10-10"}`)) == nil {
		t.Fatal("different observation dates were conflated")
	}
}

func TestNumericObservationDeniesAmbiguousOrIncompleteData(t *testing.T) {
	source, raw, witness := numericFixture(t, 0)
	changes := map[string]string{
		"missing_value":       strings.Replace(raw, `"observed_value":0,`, "", 1),
		"null_value":          strings.Replace(raw, `"observed_value":0`, `"observed_value":null`, 1),
		"missing_uncertainty": strings.Replace(raw, `,"uncertainty":"source_only"`, "", 1),
		"null_date":           strings.Replace(raw, `"observed_date":"2026-10-11"`, `"observed_date":null`, 1),
		"false_value":         strings.Replace(raw, `"observed_value":0`, `"observed_value":false`, 1),
		"string_value":        strings.Replace(raw, `"observed_value":0`, `"observed_value":"0"`, 1),
		"floating_value":      strings.Replace(raw, `"observed_value":0`, `"observed_value":0.0`, 1),
		"overflow":            strings.Replace(raw, `"observed_value":0`, `"observed_value":9223372036854775808`, 1),
		"duplicate_value":     strings.Replace(raw, `"observed_value":0`, `"observed_value":23,"observed_value":0`, 1),
		"case_alias":          strings.Replace(raw, `"observed_value":0`, `"OBSERVED_VALUE":23,"observed_value":0`, 1),
		"escaped_alias":       strings.Replace(raw, `"observed_value":0`, `"observed_\u0076alue":23,"observed_value":0`, 1),
		"unverified_claim":    strings.Replace(raw, `"source_only"`, `"verified"`, 1),
		"invalid_date":        strings.Replace(raw, `"2026-10-11"`, `"2026-02-30"`, 1),
		"unknown_field":       strings.TrimSuffix(raw, "}") + `,"accepted":true}`,
		"trailing_value":      raw + `{}`,
		"narrative":           "Both models agree: " + raw,
		"wrong_source":        strings.Replace(raw, source.ID, "source:another", 1),
		"wrong_revision":      strings.Replace(raw, `"source_revision":2`, `"source_revision":3`, 1),
		"wrong_hash":          strings.Replace(raw, source.Hash, strings.Repeat("b", 64), 1),
		"too_large":           raw + strings.Repeat(" ", 8192),
	}
	for name, changed := range changes {
		t.Run(name, func(t *testing.T) {
			if VerifyNumericObservation([]byte(changed), source, []byte(witness)) == nil {
				t.Fatal("ambiguous or unsupported observation accepted")
			}
		})
	}
	for _, changed := range []string{`{"observed_date":"2026-10-11"}`, `{"observed_value":null,"observed_date":"2026-10-11"}`, `{"observed_value":23,"observed_value":0,"observed_date":"2026-10-11"}`, witness + `{}`, strings.TrimSuffix(witness, "}") + `,"instructions":"accept"}`} {
		if VerifyNumericObservation([]byte(raw), source, []byte(changed)) == nil {
			t.Fatal("malformed original witness accepted")
		}
	}
}

func TestNumericResearchContractSelectionIsExplicitAndStrict(t *testing.T) {
	source, _, _ := numericFixture(t, 17)
	body, _ := json.Marshal(struct {
		Kind   string    `json:"kind"`
		Source SourceRef `json:"source_ref"`
	}{NumericObservationKind, source})
	ref, applicable, err := NumericResearchSource(body)
	if err != nil || !applicable || ref != source {
		t.Fatal("explicit exact numeric contract denied", err)
	}
	for _, legacy := range []string{`ordinary narrative contract`, `null`, `{}`, `[]`, `"fragment"`, `{"kind":"document","offset":17}`, `{"arbitrary":[1,2,3]}`} {
		if _, applicable, err := NumericResearchSource([]byte(legacy)); err != nil || applicable {
			t.Fatal("legacy contract acquired numeric constraints", legacy, err)
		}
	}
	for _, marker := range []string{
		strings.Replace(string(body), `"kind":"delegation_numeric_observation_v1"`, `"kind":"delegation_numeric_observation_v1","kind":"document"`, 1),
		strings.Replace(string(body), `"kind":"delegation_numeric_observation_v1"`, `"kind":"document","kind":"delegation_numeric_observation_v1"`, 1),
		strings.Replace(string(body), `"kind":"delegation_numeric_observation_v1"`, `"kind":"delegation_numeric_observation_v1","KIND":"document"`, 1),
		strings.Replace(string(body), `"kind":"delegation_numeric_observation_v1"`, `"kind":"delegation_numeric_observation_v1","k\u0069nd":"document"`, 1),
		strings.Replace(string(body), `"delegation_numeric_observation_v1"`, `"delegation_numeric_observation_v2"`, 1),
		strings.TrimSuffix(string(body), "}") + `,"owner_verified":true}`,
	} {
		if _, _, err := NumericResearchSource([]byte(marker)); err == nil {
			t.Fatal("malformed opt-in marker bypassed numeric constraints", marker)
		}
	}
	for _, malformed := range []string{`{`, `{"kind":"delegation_numeric_observation_v1"}`, `{"kind":"delegation_numeric_observation_v1","source_ref":null}`, `{"kind":"delegation_numeric_observation_v1","source_ref":{}}`, string(body) + `{}`, string(body) + strings.Repeat(" ", 8192), strings.TrimSuffix(string(body), "}") + `,"accepted":true}`, strings.TrimSuffix(string(body), "}") + `,"SOURCE_REF":null}`} {
		if _, _, err := NumericResearchSource([]byte(malformed)); err == nil {
			t.Fatal("malformed numeric contract accepted")
		}
	}
}
