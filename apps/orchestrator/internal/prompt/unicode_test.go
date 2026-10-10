package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPromptTruncationPreservesUnicode(t *testing.T) {
	value := strings.Repeat("Уточнение🦉", 100)
	for _, limit := range []int{1, 2, 11, 20, 700} {
		got, truncated := truncate(value, limit)
		if !truncated || !utf8.ValidString(got) || utf8.RuneCountInString(got) > limit {
			t.Fatalf("limit %d: invalid prompt truncation", limit)
		}
	}
}
