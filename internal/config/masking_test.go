package config

import "testing"

func TestMaskForDisplayReturnsEmptyForUnsetValues(t *testing.T) {
	if got := MaskForDisplay("BUTLER_OPENAI_API_KEY", ""); got != "" {
		t.Fatalf("expected empty mask for unset value, got %q", got)
	}
}

func TestMaskForDisplayFullyMasksPasswordsAndConnectionStrings(t *testing.T) {
	for _, testCase := range []struct {
		key   string
		value string
	}{
		{key: "BUTLER_ADMIN_PASSWORD", value: "hunter2"},
		{key: "BUTLER_POSTGRES_URL", value: "postgres://user:pass@localhost:5432/butler"},
	} {
		if got := MaskForDisplay(testCase.key, testCase.value); got != "••••••••" {
			t.Fatalf("expected full mask for %s, got %q", testCase.key, got)
		}
	}
}

func TestMaskForDisplayShowsLastFourForAPIKeys(t *testing.T) {
	if got := MaskForDisplay("BUTLER_OPENAI_API_KEY", "sk-abcdefghijkl1234"); got != "...1234" {
		t.Fatalf("expected last four mask, got %q", got)
	}
}

func TestMaskForDisplayShowsTelegramPrefixAndSuffix(t *testing.T) {
	if got := MaskForDisplay("BUTLER_TELEGRAM_BOT_TOKEN", "123456:ABCDEFxyz"); got != "123456:...xyz" {
		t.Fatalf("expected telegram token mask, got %q", got)
	}
}
