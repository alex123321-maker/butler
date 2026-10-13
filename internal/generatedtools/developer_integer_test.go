package generatedtools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
)

func developerIntegerFixture(t *testing.T) string {
	t.Helper()
	a := DeveloperArgs{NeedSource: catalog.SourceRef{ID: "need", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}, ContractSource: catalog.SourceRef{ID: "spec", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("b", 64)}, Budget: budget.Units{ModelTurns: 2, Tokens: 4096, ExternalCalls: 4, ActiveMillis: 1320000, EgressBytes: 4096, ProviderInputBytes: 4096}, Deadline: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Dependencies: []string{}}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func TestDeveloperArgsLosslessIntegerBoundary(t *testing.T) {
	base := developerIntegerFixture(t)
	for _, value := range []string{"2.0", "2e0", "200e-2"} {
		t.Run(value, func(t *testing.T) {
			raw := strings.Replace(base, `"model_turns":2`, `"model_turns":`+value, 1)
			before, _ := actionauth.ArgumentsHash(raw)
			args, err := DecodeDeveloperArgs(raw)
			after, _ := actionauth.ArgumentsHash(raw)
			if err != nil || args.Budget.ModelTurns != 2 || before != after {
				t.Fatal("exact equivalent integer denied or proposal changed", err)
			}
		})
	}
	t.Run("exact_above_float_precision", func(t *testing.T) {
		raw := strings.Replace(base, `"tokens":4096`, `"tokens":9007199254740993.0`, 1)
		args, err := DecodeDeveloperArgs(raw)
		if err != nil || args.Budget.Tokens != 9007199254740993 {
			t.Fatal("integer rounded through floating point", err)
		}
	})
	t.Run("reference_integer_equivalence", func(t *testing.T) {
		raw := strings.ReplaceAll(base, `"revision":1`, `"revision":1.0`)
		raw = strings.ReplaceAll(raw, `"access_revision":1`, `"access_revision":1e0`)
		args, err := DecodeDeveloperArgs(raw)
		if err != nil || args.NeedSource.Revision != 1 || args.ContractSource.AccessRevision != 1 {
			t.Fatal("source numeric identity changed", err)
		}
	})
}
func TestDeveloperArgsIntegerAndStructureDenials(t *testing.T) {
	base := developerIntegerFixture(t)
	for _, value := range []string{"1.5", "null", `"2"`, "-1", "9223372036854775808", strings.Repeat("9", 513), "1e1025", "1e-1025"} {
		t.Run("value_"+strings.ReplaceAll(value[:min(16, len(value))], "\"", "quoted"), func(t *testing.T) {
			raw := strings.Replace(base, `"model_turns":2`, `"model_turns":`+value, 1)
			if _, err := DecodeDeveloperArgs(raw); err == nil {
				t.Fatal("fraction, unknown, overflow or unbounded integer admitted")
			}
		})
	}
	for _, tc := range []struct{ name, raw string }{
		{"missing_field", strings.Replace(base, `"children":0,`, "", 1)},
		{"unknown_budget_field", strings.Replace(base, `"model_turns":2`, `"unknown":0,"model_turns":2`, 1)},
		{"duplicate_budget_field", strings.Replace(base, `"model_turns":2`, `"model_turns":2,"model_turns":2.0`, 1)},
		{"extra_outer_field", strings.TrimSuffix(base, "}") + `,"permissions":[]}`},
		{"fractional_source", strings.Replace(base, `"revision":1`, `"revision":1.5`, 1)},
		{"duplicate_source", strings.Replace(base, `"revision":1`, `"revision":1,"revision":1.0`, 1)},
		{"extra_packet", base + `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeDeveloperArgs(tc.raw); err == nil {
				t.Fatal("ambiguous or expanded developer proposal admitted")
			}
		})
	}
}
