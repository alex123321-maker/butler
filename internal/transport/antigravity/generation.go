package antigravity

import (
	"encoding/json"
	"regexp"
	"strconv"
)

// Generation is trusted construction-time configuration for explicit paid Flash
// profiles. No fallback or in-flight mutation; semantic validation stays owned
// by the caller. Empty ThinkingLevel preserves the existing automatic contract.
func validateGenerationConfig(cfg *Config) error {
	cloudProfile := cfg.Model == FlashHighModel || cfg.Model == FlashLowModel
	if cfg.ThinkingLevel != "" && !(cfg.Model == FlashLowModel && cfg.ThinkingLevel == "LOW" || cfg.Model == FlashHighModel && cfg.ThinkingLevel == "HIGH") {
		return protocolError("unsupported explicit thinking level")
	}
	if cfg.MaximumOutputTokens == 0 {
		cfg.MaximumOutputTokens = maxOutputTokens
	}
	structured := len(cfg.JSONResponseSchema) > 0 || len(cfg.GoogleResponseSchema) > 0
	if cfg.MaximumOutputTokens < 1 || cfg.MaximumOutputTokens > 32768 ||
		cfg.MaximumOutputTokens > maxOutputTokens && cfg.Model != FlashHighModel || structured && !cloudProfile {
		return protocolError("unsupported configured generation allowance")
	}
	if len(cfg.JSONResponseSchema) > 0 && len(cfg.GoogleResponseSchema) > 0 {
		return protocolError("configured response schema dialects are mutually exclusive")
	}
	if len(cfg.GoogleResponseSchema) > 0 {
		if len(cfg.GoogleResponseSchema) > 16384 {
			return protocolError("configured Google schema exceeds its byte limit")
		}
		decoded, err := strictJSON(cfg.GoogleResponseSchema)
		object, ok := decoded.(map[string]any)
		nodes := 0
		if err != nil || !ok || object["type"] != "OBJECT" || !inspectGoogleResponseSchema(object, 0, &nodes) {
			return protocolError("unsupported configured Google response schema")
		}
		cfg.GoogleResponseSchema = append(json.RawMessage(nil), cfg.GoogleResponseSchema...)
		return nil
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

var googleSchemaPropertyName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// This deliberately small native Schema subset has no references, annotations,
// unions, defaults or additionalProperties. It specifies syntax only. The caller
// still rejects extra fields, wrong sources and incorrect semantic answers.
func inspectGoogleResponseSchema(value any, depth int, nodes *int) bool {
	node, ok := value.(map[string]any)
	if !ok || depth > 8 || *nodes >= 256 {
		return false
	}
	*nodes = *nodes + 1
	kind, ok := node["type"].(string)
	if !ok {
		return false
	}
	allowed := map[string]bool{"type": true, "nullable": true}
	if nullable, present := node["nullable"]; present {
		if _, ok := nullable.(bool); !ok {
			return false
		}
	}
	switch kind {
	case "OBJECT":
		allowed["properties"], allowed["required"] = true, true
		props, ok := node["properties"].(map[string]any)
		if !ok || len(props) == 0 || len(props) > 32 {
			return false
		}
		for name, property := range props {
			if !googleSchemaPropertyName.MatchString(name) || !inspectGoogleResponseSchema(property, depth+1, nodes) {
				return false
			}
		}
		if required, present := node["required"]; present {
			items, ok := required.([]any)
			if !ok || len(items) > len(props) {
				return false
			}
			seen := map[string]bool{}
			for _, item := range items {
				name, ok := item.(string)
				if !ok || props[name] == nil || seen[name] {
					return false
				}
				seen[name] = true
			}
		}
	case "ARRAY":
		allowed["items"], allowed["minItems"], allowed["maxItems"] = true, true, true
		maximum, valid := googleSchemaArrayBound(node["maxItems"])
		if !valid || maximum == 0 || !inspectGoogleResponseSchema(node["items"], depth+1, nodes) {
			return false
		}
		if raw, present := node["minItems"]; present {
			minimum, valid := googleSchemaArrayBound(raw)
			if !valid || minimum > maximum {
				return false
			}
		}
	case "STRING":
		allowed["enum"] = true
		if raw, present := node["enum"]; present {
			items, ok := raw.([]any)
			if !ok || len(items) == 0 || len(items) > 64 {
				return false
			}
			seen := map[string]bool{}
			for _, raw := range items {
				text, ok := raw.(string)
				if !ok || len(text) == 0 || len(text) > 256 || seen[text] {
					return false
				}
				seen[text] = true
			}
		}
	case "BOOLEAN", "INTEGER", "NUMBER":
	default:
		return false
	}
	for key := range node {
		if !allowed[key] {
			return false
		}
	}
	return true
}

// Native Schema's int64 JSON fields use decimal strings. Require bounded,
// canonical forms rather than accepting ambiguous numeric or exponent spellings.
func googleSchemaArrayBound(value any) (int, bool) {
	text, ok := value.(string)
	if !ok || len(text) == 0 || len(text) > 3 {
		return 0, false
	}
	count, err := strconv.Atoi(text)
	return count, err == nil && count >= 0 && count <= 128 && strconv.Itoa(count) == text
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
