package app

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestTemporalQualificationRemainsSeparateFromProcessFlag(t *testing.T) {
	if temporalConfigured(false) || temporalConfigured(true) {
		t.Fatal("unqualified temporal feature became active")
	}
	for _, status := range []string{"unknown", "unverified", "needs_revalidation", "stable", "fresh", "historical"} {
		if temporalExcluded(catalog.AcceptedFact{Freshness: catalog.Freshness{Status: status}}) {
			t.Fatal("freshness erased historical or unknown evidence", status)
		}
	}
	for _, status := range []string{"expired", "not_yet_valid"} {
		if !temporalExcluded(catalog.AcceptedFact{Freshness: catalog.Freshness{Status: status}}) {
			t.Fatal("invalid current exception remained eligible", status)
		}
	}
}

func TestTemporalFactValueSafetyPreservesNumbersAndRejectsCredentialFields(t *testing.T) {
	if !temporalFactValueSafe(map[string]any{"value": json.Number("9007199254740993")}) {
		t.Fatal("precise accepted number rejected")
	}
	for _, v := range []any{map[string]any{"access_token": "private"}, map[string]any{"value": "Bearer synthetic-private-value"}, map[string]any{"nested": []any{map[string]any{"password": "private"}}}} {
		if temporalFactValueSafe(v) {
			t.Fatal("credential-bearing accepted value released")
		}
	}
}
