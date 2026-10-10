package broker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestReflectedCredentialIsMaskedBeforeModelBoundary(t *testing.T) {
	secret := "private\"token\n123"
	encoded, _ := json.Marshal(map[string]any{"echo": "Bearer " + secret, "nested": []any{map[string]string{"value": secret}}, "integer": json.Number("9007199254740993")})
	original := &toolbrokerv1.ToolResult{ResultJson: string(encoded), Error: &toolbrokerv1.ToolError{Message: secret, DetailsJson: string(encoded)}}
	clean := redactResult(original, []credentials.ResolvedSecret{{Alias: "account", Field: "token", Value: secret}})
	var object map[string]any
	if json.Unmarshal([]byte(clean.ResultJson), &object) != nil {
		t.Fatal("redaction corrupted JSON")
	}
	if strings.Contains(clean.Error.Message, "private") || strings.Contains(clean.ResultJson, "private") || !strings.Contains(clean.ResultJson, "9007199254740993") {
		t.Fatal("credential leaked or unrelated number changed")
	}
	if original.Error.Message != secret {
		t.Fatal("runtime result was mutated")
	}
}
