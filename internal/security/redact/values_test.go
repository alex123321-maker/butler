package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateValueCannotEscapeReflectionMaskThroughJSONEncoding(t *testing.T) {
	secret := "synthetic \"private\"\ncredential & value"
	raw, _ := json.Marshal(map[string]string{"body": secret})
	masked := Values(string(raw), secret)
	var result map[string]string
	if json.Unmarshal([]byte(masked), &result) != nil || result["body"] != "[REDACTED]" || strings.Contains(masked, "private") {
		t.Fatal("JSON reflection leaked resolved value or corrupted result")
	}
}
