package acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// DecodeStrict validates the runner envelope without rewriting raw model output.
// Nested typed measurements perform their own presence/duplicate checks. The
// scorer still sees duplicate keys in a model response and counts that error.
func DecodeStrict(raw []byte, out any, fields ...string) error {
	_, err := decodeMeasurement(raw, out, fields...)
	return err
}

func measurementFields(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("measurement object required")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("measurement key required")
		}
		if _, exists := fields[key]; exists {
			return nil, errors.New("duplicate measurement field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing or invalid measurement object")
	}
	return fields, nil
}

func decodeMeasurement(raw []byte, out any, required ...string) (map[string]json.RawMessage, error) {
	fields, err := measurementFields(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok || key != "output" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("missing measurement field: %s", key)
		}
	}
	for key := range fields {
		known := false
		for _, expected := range required {
			known = known || key == expected
		}
		if !known {
			return nil, errors.New("unknown measurement field")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return nil, err
	}
	return fields, nil
}

func (o *Observation) UnmarshalJSON(raw []byte) error {
	type plain Observation
	var value plain
	_, err := decodeMeasurement(raw, &value, "case_id", "output", "milliseconds", "input_tokens", "output_tokens")
	if err != nil {
		return err
	}
	// Actual null model output remains an incorrect response; absent output
	// and missing/null measurement metadata have already been denied.
	*o = Observation(value)
	return nil
}

func (m *TaskMeasurement) UnmarshalJSON(raw []byte) error {
	type plain TaskMeasurement
	var value plain
	_, err := decodeMeasurement(raw, &value, "id", "verified_success", "missed_important", "false_initiatives", "permission_errors", "repeated_effects", "source_errors", "milliseconds", "model_requests", "input_tokens", "output_tokens", "tool_calls", "child_tasks", "cost_microunits", "memory_peak_bytes")
	if err != nil {
		return err
	}
	*m = TaskMeasurement(value)
	return nil
}

func (p *ModelPin) UnmarshalJSON(raw []byte) error {
	type plain ModelPin
	var value plain
	_, err := decodeMeasurement(raw, &value, "model", "weights_sha256", "runtime", "hardware_sha256", "corpus_digest", "mode")
	if err != nil {
		return err
	}
	*p = ModelPin(value)
	return nil
}

func (r *ComparisonRun) UnmarshalJSON(raw []byte) error {
	type plain ComparisonRun
	var value plain
	_, err := decodeMeasurement(raw, &value, "mode", "code_sha256", "tasks_sha256", "resources_sha256", "model", "pricing", "outcomes")
	if err != nil {
		return err
	}
	*r = ComparisonRun(value)
	return nil
}
