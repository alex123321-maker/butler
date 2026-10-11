package generatedtools

import (
	"bytes"
	"encoding/json"
)

// Broker's privacy boundary decodes a tool result and re-encodes JSON objects
// with sorted keys before journaling its digest. Emit the same representation
// for typed converter observations; struct field order is not durable proof.
// This operates only on kernel-produced values, never on unvalidated model JSON.
func observationJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object any
	if err = decoder.Decode(&object); err != nil {
		return "", err
	}
	raw, err = json.Marshal(object)
	return string(raw), err
}

func observationHash(value any) (string, error) {
	raw, err := observationJSON(value)
	if err != nil {
		return "", err
	}
	return HashBytes([]byte(raw)), nil
}
