package factcontrol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTemporalFactsStrictSourceDateAndBoundContract(t *testing.T) {
	valid := `{"sources":[{"source_id":"source_1","revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("a", 64) + `"}],"valid_at":"2026-02-15T00:00:00Z","known_at":"2026-03-01T00:00:00+07:00"}`
	a, err := DecodeArgs(valid)
	if err != nil || a.Limit != 8 || a.ValidAt == a.KnownAt {
		t.Fatal(a, err)
	}
	for _, raw := range []string{strings.Replace(valid, `"revision":1`, `"revision":1,"revision":2`, 1), strings.Replace(valid, `2026-02-15T00:00:00Z`, `2026-02-15`, 1), strings.Replace(valid, `2026-02-15T00:00:00Z`, `tomorrow`, 1), strings.Replace(valid, `2026-02-15T00:00:00Z`, `2026-02-15T00:00:00`, 1), strings.Replace(valid, `"sources":`, `"owner_id":"forged","sources":`, 1), strings.Replace(valid, `"revision":1`, `"revision":0`, 1), strings.Replace(valid, `"valid_at":`, `"limit":11,"valid_at":`, 1)} {
		if _, err := DecodeArgs(raw); err == nil {
			t.Fatal("ambiguous/unbounded temporal request admitted")
		}
	}
	if ReleaseQualified {
		t.Fatal("unmeasured temporal profile qualified")
	}
	var schema any
	if json.Unmarshal([]byte(Contract().InputSchemaJson), &schema) != nil {
		t.Fatal("invalid canonical schema")
	}
	if Contract().ToolName != "memory.facts" || Contract().ContractRevision != "5" {
		t.Fatal("temporal lookup changed another read contract")
	}
}

func TestTemporalFactsPrivateEndpointRejectsCredentialAndRedirectShapes(t *testing.T) {
	for _, endpoint := range []string{"http://example.com/internal/memory-facts", "http://user:private@127.0.0.1/internal/memory-facts", "http://127.0.0.1/internal/memory-facts?private=1", "http://orchestrator:8080/internal/memory-read"} {
		if _, err := NewClient(endpoint, strings.Repeat("t", 40)); err == nil {
			t.Fatal("untrusted private route accepted")
		}
	}
	if _, err := NewClient("http://orchestrator:8080/internal/memory-facts", strings.Repeat("t", 40)); err != nil {
		t.Fatal(err)
	}
}
