package app

import "testing"

func TestGuardedTabOriginPolicyIsExplicit(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `["https://example.invalid/"]`, `["https://user:pass@example.invalid"]`, `["https://example.invalid","https://example.invalid"]`, `["https://example.invalid?q=x"]`, `["HTTPS://example.invalid"]`} {
		if _, err := guardedOrigins(raw); err == nil {
			t.Fatal("unreviewed origin policy accepted", raw)
		}
	}
	if out, err := guardedOrigins(`["https://example.invalid","http://127.0.0.1:12345"]`); err != nil || len(out) != 2 {
		t.Fatal("exact operator origins denied", err)
	}
}
