package sanitize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMaskCredentialsKeepsOriginalUnicodeOffsets(t *testing.T) {
	for _, value := range []string{"Bearer SYNTHETIC_TOKEN_12345", "password=синтетический_пароль", "postgres://user:synthetic@local/db", "ghp_SYNTHETIC123456789"} {
		prefix, suffix := "Пролог: ", " конец"
		raw := prefix + value + suffix
		masked := MaskCredentials(raw)
		if masked == raw || utf8.RuneCountInString(masked) != utf8.RuneCountInString(raw) || !strings.HasPrefix(masked, prefix) || !strings.HasSuffix(masked, suffix) {
			t.Fatal("mask did not preserve a safe positional view")
		}
		if strings.Contains(masked, "SYNTHETIC") || strings.Contains(masked, "синтетический") {
			t.Fatal("recognized credential stayed visible")
		}
	}
	if got := MaskCredentials("Обычный проверяемый документ"); got != "Обычный проверяемый документ" {
		t.Fatal("unrelated evidence changed")
	}
}
