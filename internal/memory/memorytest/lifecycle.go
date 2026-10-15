package memorytest

import (
	_ "embed"
	"encoding/json"
	"testing"
)

// LifecycleAttack is synthetic untrusted DATA. Expected records an observable
// kernel decision, never a model's assertion that it followed instructions.
type LifecycleAttack struct {
	ID         string `json:"id"`
	Vector     string `json:"vector"`
	OriginKind string `json:"origin_kind"`
	Body       string `json:"body"`
	Expected   string `json:"expected"`
}

//go:embed testdata/lifecycle-v1.json
var lifecycleCorpus []byte

// LifecycleAttacks supplies the same public corpus to every consumer's tests.
// A consumer must also exercise its own publication/read/invalidation boundary;
// passing this loader or a provenance-only test is not lifecycle acceptance.
func LifecycleAttacks(t testing.TB) []LifecycleAttack {
	t.Helper()
	var cases []LifecycleAttack
	if err := json.Unmarshal(lifecycleCorpus, &cases); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] || c.Vector == "" || c.OriginKind == "" || c.Body == "" || c.Expected != "data_only_no_authority" {
			t.Fatal("invalid lifecycle attack corpus")
		}
		seen[c.ID] = true
	}
	if len(cases) < 8 {
		t.Fatal("incomplete lifecycle attack corpus")
	}
	return cases
}
