package deliveryoutbox

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestTelegramChunksPreserveUnicodeAndUTF16Limit(t *testing.T) {
	text := strings.Repeat("🙂aЖ\n", 2500)
	parts := Split(text, 4096)
	if strings.Join(parts, "") != text || len(parts) < 2 {
		t.Fatal("content lost or not split")
	}
	for _, part := range parts {
		if !utf8.ValidString(part) || len(utf16.Encode([]rune(part))) > 4096 {
			t.Fatal("invalid Telegram chunk")
		}
	}
}
