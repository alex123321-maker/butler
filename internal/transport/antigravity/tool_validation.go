package antigravity

import "encoding/json"

func ValidateLogicalToolSchema(schema string) error {
	_, err := nativeToolSchema(schema)
	return err
}

func ValidateLogicalToolArguments(schema string, arguments json.RawMessage) error {
	return validateArguments(schema, arguments)
}
