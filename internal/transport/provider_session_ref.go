package transport

import (
	"encoding/json"
	"fmt"
	"strings"
)

func MarshalProviderSessionRef(ref *ProviderSessionRef) (string, error) {
	if ref == nil {
		return "", nil
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		return "", fmt.Errorf("marshal provider session ref: %w", err)
	}
	return string(encoded), nil
}

func MustMarshalProviderSessionRef(ref *ProviderSessionRef) string {
	encoded, err := MarshalProviderSessionRef(ref)
	if err != nil {
		return ""
	}
	return encoded
}

func ParseProviderSessionRef(value string) (*ProviderSessionRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var ref ProviderSessionRef
	if err := json.Unmarshal([]byte(value), &ref); err != nil {
		return nil, fmt.Errorf("parse provider session ref: %w", err)
	}
	return &ref, nil
}
