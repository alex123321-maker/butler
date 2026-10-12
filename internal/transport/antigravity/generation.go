package antigravity

import (
	"encoding/json"
)

// Configure only the current reviewed High profile. No fallback or mutation of
// in-flight generation is supported. Schema validation remains application-owned.
func validateGenerationConfig(cfg *Config) error {
	if cfg.MaximumOutputTokens == 0 {
		cfg.MaximumOutputTokens = maxOutputTokens
	}
	if cfg.MaximumOutputTokens < 1 || cfg.MaximumOutputTokens > 32768 ||
		(cfg.MaximumOutputTokens > maxOutputTokens || len(cfg.JSONResponseSchema) > 0) && cfg.Model != FlashHighModel {
		return protocolError("unsupported configured generation allowance")
	}
	if len(cfg.JSONResponseSchema) == 0 {
		return nil
	}
	if len(cfg.JSONResponseSchema) > 16384 {
		return protocolError("configured JSON schema exceeds its byte limit")
	}
	decoded, err := strictJSON(cfg.JSONResponseSchema)
	object, ok := decoded.(map[string]any)
	if err != nil || !ok || object["type"] != "object" || schemaHasReference(object) {
		return protocolError("unsupported configured JSON schema")
	}
	// Copy the trusted source so a caller cannot change a frozen configuration.
	cfg.JSONResponseSchema = append(json.RawMessage(nil), cfg.JSONResponseSchema...)
	return nil
}

func schemaHasReference(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == "$ref" || schemaHasReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if schemaHasReference(child) {
				return true
			}
		}
	}
	return false
}
