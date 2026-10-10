package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// This is the JSON Schema subset used by Butler's tool catalog, not a policy
// engine. Broker validates the contract again and remains the action authority.
func validateSchema(text string) error {
	value, err := strictJSON([]byte(text))
	if err != nil {
		return err
	}
	return inspectSchema(value, 0)
}
func inspectSchema(value any, depth int) error {
	schema, ok := value.(map[string]any)
	if !ok || depth > 32 {
		return fmt.Errorf("invalid schema")
	}
	for key, value := range schema {
		switch key {
		case "type":
			typ, ok := value.(string)
			if !ok || !strings.Contains("|object|string|boolean|number|integer|array|null|", "|"+typ+"|") {
				return fmt.Errorf("unsupported type")
			}
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid properties")
			}
			for _, property := range props {
				if err := inspectSchema(property, depth+1); err != nil {
					return err
				}
			}
		case "required":
			items, ok := value.([]any)
			if !ok {
				return fmt.Errorf("invalid required")
			}
			for _, name := range items {
				if _, ok := name.(string); !ok {
					return fmt.Errorf("invalid required name")
				}
			}
		case "items":
			if err := inspectSchema(value, depth+1); err != nil {
				return err
			}
		case "oneOf", "anyOf":
			items, ok := value.([]any)
			if !ok || len(items) == 0 {
				return fmt.Errorf("invalid union")
			}
			for _, item := range items {
				if err := inspectSchema(item, depth+1); err != nil {
					return err
				}
			}
		case "additionalProperties":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("unsupported additional properties")
			}
		case "enum":
			if items, ok := value.([]any); !ok || len(items) == 0 {
				return fmt.Errorf("invalid enum")
			}
		case "description", "title", "$schema":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("invalid annotation")
			}
		case "default": // Annotation only.
		default:
			return fmt.Errorf("unsupported schema keyword")
		}
	}
	return nil
}
func validateArguments(schemaJSON string, raw json.RawMessage) error {
	value, err := strictJSON(raw)
	if err != nil {
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("function arguments must be an object")
	}
	schema, err := strictJSON([]byte(schemaJSON))
	if err != nil {
		return err
	}
	return validateValue(value, schema.(map[string]any))
}
func validateValue(value any, schema map[string]any) error {
	for _, kind := range []string{"oneOf", "anyOf"} {
		if candidates, ok := schema[kind].([]any); ok {
			matches := 0
			for _, candidate := range candidates {
				if validateValue(value, candidate.(map[string]any)) == nil {
					matches++
				}
			}
			if matches == 0 || (kind == "oneOf" && matches != 1) {
				return fmt.Errorf("invalid union value")
			}
		}
	}
	typ, _ := schema["type"].(string)
	if typ == "" && schema["properties"] != nil {
		typ = "object"
	}
	switch typ {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		props, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, name := range required {
				if _, ok := object[name.(string)]; !ok {
					return fmt.Errorf("missing field")
				}
			}
		}
		for name, item := range object {
			if property, ok := props[name]; ok {
				if err := validateValue(item, property.(map[string]any)); err != nil {
					return err
				}
			} else if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
				return fmt.Errorf("unknown field")
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean")
		}
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("expected number")
		}
		if typ == "integer" {
			rational, ok := new(big.Rat).SetString(number.String())
			if !ok || !rational.IsInt() {
				return fmt.Errorf("expected integer")
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for _, item := range items {
				if err := validateValue(item, itemSchema); err != nil {
					return err
				}
			}
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null")
		}
	}
	if candidates, ok := schema["enum"].([]any); ok {
		for _, candidate := range candidates {
			if reflect.DeepEqual(value, candidate) {
				return nil
			}
		}
		return fmt.Errorf("invalid enum value")
	}
	return nil
}

// Reject duplicate keys before Go's map decoder can erase ambiguity. Preserve
// JSON number precision when forwarding tool arguments/results.
func strictJSON(data []byte) (any, error) {
	if len(data) == 0 || len(data) > 1024*1024 {
		return nil, fmt.Errorf("invalid JSON size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := readJSON(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	return value, nil
}
func readJSON(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid JSON key")
			}
			if _, duplicate := object[name]; duplicate {
				return nil, fmt.Errorf("duplicate JSON key")
			}
			value, err := readJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, fmt.Errorf("invalid JSON object")
		}
		return object, nil
	case '[':
		items := []any{}
		for decoder.More() {
			value, err := readJSON(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, fmt.Errorf("invalid JSON array")
		}
		return items, nil
	default:
		return nil, fmt.Errorf("invalid JSON delimiter")
	}
}
