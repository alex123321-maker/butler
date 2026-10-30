package transport

import (
	"encoding/json"
	"testing"
)

func TestUsageDimensionsCompareValuesAfterDecode(t *testing.T) {
	var a, b Usage
	raw := []byte(`{"InputTokens":12,"OutputTokens":8,"TotalTokens":20,"CachedInputTokens":4,"CacheWriteInputTokens":7,"CacheWrite30mTokens":3,"CacheWrite12hTokens":4,"OutputReasoningTokens":5}`)
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal(raw, &b) != nil || a.Validate() != nil || b.Validate() != nil || !a.Equal(b) {
		t.Fatal("equal decoded dimensions conflict")
	}
	*b.CachedInputTokens = 3
	if a.Equal(b) {
		t.Fatal("different dimensions accepted")
	}
	zero := 0
	a.CachedInputTokens = &zero
	b = a
	b.CachedInputTokens = nil
	if a.Equal(b) {
		t.Fatal("unknown collapsed into zero")
	}
}
