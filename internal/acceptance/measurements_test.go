package acceptance

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRunnerMeasurementsRequirePresenceNotInferredZeros(t *testing.T) {
	observation := `{"case_id":"case","output":{"facts":[]},"milliseconds":0,"input_tokens":0,"output_tokens":0}`
	var actual Observation
	if err := json.Unmarshal([]byte(observation), &actual); err != nil {
		t.Fatal("explicit measured zeros denied", err)
	}
	for _, key := range []string{"case_id", "output", "milliseconds", "input_tokens", "output_tokens"} {
		t.Run("observation_"+key, func(t *testing.T) {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal([]byte(observation), &fields)
			delete(fields, key)
			raw, _ := json.Marshal(fields)
			if json.Unmarshal(raw, &actual) == nil {
				t.Fatal("absent field became a measurement")
			}
			fields[key] = json.RawMessage("null")
			raw, _ = json.Marshal(fields)
			err := json.Unmarshal(raw, &actual)
			if key == "output" {
				if err != nil || string(actual.Output) != "null" {
					t.Fatal("actual null model answer lost", err)
				}
			} else if err == nil {
				t.Fatal("null metadata became a measurement")
			}
		})
	}
	measured := TaskMeasurement{ID: "task", Milliseconds: 1, ModelRequests: 1, MemoryPeakBytes: 1}
	raw, _ := json.Marshal(measured)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var actualTask TaskMeasurement
	if err := json.Unmarshal(raw, &actualTask); err != nil || actualTask != measured {
		t.Fatal("explicit false/zero measurements denied", err)
	}
	for key, original := range fields {
		delete(fields, key)
		raw, _ = json.Marshal(fields)
		if json.Unmarshal(raw, &actualTask) == nil {
			t.Fatalf("missing task measurement %s accepted", key)
		}
		fields[key] = json.RawMessage("null")
		raw, _ = json.Marshal(fields)
		if json.Unmarshal(raw, &actualTask) == nil {
			t.Fatalf("null task measurement %s accepted", key)
		}
		fields[key] = original
	}
}

func TestTokenSubtotalsCannotOverflowIntoAFalseZero(t *testing.T) {
	c := corpusFixture(t)
	pin := ModelPin{Model: "fixture", WeightsSHA256: strings.Repeat("a", 64), Runtime: "fixture", HardwareSHA256: strings.Repeat("b", 64), CorpusDigest: CorpusDigest(c), Mode: "observation_only"}
	var observations []Observation
	for _, e := range c.Examples {
		if e.Split == "holdout" && !e.NoModel {
			observations = append(observations, Observation{CaseID: e.ID, Output: e.Golden, InputTokens: math.MaxInt64})
		}
	}
	if _, err := Score(c, pin, observations, "holdout"); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatal("overflowed token sum accepted", err)
	}
}

func TestRunnerEnvelopeRejectsConflictingMetadataAndRetainsModelError(t *testing.T) {
	var value struct {
		Observations []Observation `json:"observations"`
	}
	valid := `{"observations":[{"case_id":"case","output":{"intent":"send","intent":"stop_tree"},"milliseconds":1,"input_tokens":0,"output_tokens":0}]}`
	if err := DecodeStrict([]byte(valid), &value, "observations"); err != nil {
		t.Fatal("duplicate-key model answer must remain an observable error", err)
	}
	if _, err := canonicalJSON(value.Observations[0].Output); err == nil {
		t.Fatal("model command duplicate lost during metadata decode")
	}
	for _, bad := range []string{
		`{"observations":[],"observations":[]}`,
		strings.Replace(valid, `"milliseconds":1`, `"milliseconds":1,"milliseconds":0`, 1),
		strings.Replace(valid, `"input_tokens":0`, `"input_tokens":0,"unmeasured":true`, 1),
		strings.Replace(valid, `"input_tokens":0`, `"input_tokens":0,"Input_tokens":4`, 1),
		strings.Replace(valid, `"observations":`, `"Observations":`, 1),
		valid + `{}`,
		`null`,
	} {
		if DecodeStrict([]byte(bad), &value, "observations") == nil {
			t.Fatal("conflicting, unknown or trailing measurement metadata accepted")
		}
	}
	var pin ModelPin
	if json.Unmarshal([]byte(`{"model":"one","model":"two","weights_sha256":"a","runtime":"r","hardware_sha256":"b","corpus_digest":"c","mode":"observation_only"}`), &pin) == nil {
		t.Fatal("duplicate pin accepted")
	}
}
