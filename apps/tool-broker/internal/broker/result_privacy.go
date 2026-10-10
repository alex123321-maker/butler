package broker

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

// A runtime may receive secrets, but its return value crosses into the model.
// Mask known resolved values even when a remote server reflects them back.
func redactResult(result *toolbrokerv1.ToolResult, secrets []credentials.ResolvedSecret) *toolbrokerv1.ToolResult {
	if result == nil {
		return nil
	}
	copy := proto.Clone(result).(*toolbrokerv1.ToolResult)
	replace := func(value string) string {
		for _, secret := range secrets {
			if secret.Value == "" {
				continue
			}
			for _, variant := range []string{secret.Value, url.QueryEscape(secret.Value), url.PathEscape(secret.Value), base64.StdEncoding.EncodeToString([]byte(secret.Value))} {
				if variant != "" {
					value = strings.ReplaceAll(value, variant, "[REDACTED]")
				}
			}
		}
		return value
	}
	redactJSON := func(raw string) string {
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return replace(raw)
		}
		var walk func(any) any
		walk = func(value any) any {
			switch value := value.(type) {
			case string:
				return replace(value)
			case []any:
				for i, item := range value {
					value[i] = walk(item)
				}
				return value
			case map[string]any:
				clean := map[string]any{}
				for key, item := range value {
					clean[replace(key)] = walk(item)
				}
				return clean
			default:
				return value
			}
		}
		encoded, err := json.Marshal(walk(value))
		if err != nil {
			return "{}"
		}
		return string(encoded)
	}
	copy.ResultJson = redactJSON(copy.ResultJson)
	if copy.Error != nil {
		copy.Error.Message = replace(copy.Error.Message)
		copy.Error.DetailsJson = redactJSON(copy.Error.DetailsJson)
	}
	return copy
}
