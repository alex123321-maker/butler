package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfilePromptPreservesProvenanceAcrossJSONRoundTrip(t *testing.T) {
	entries := []map[string]any{
		{"key": "style", "summary": "Подробно только здесь", "scope_type": "session", "scope_id": "session\n\"quoted\"", "source_refs": []string{"source:\"original\""}},
		{"key": "legacy", "summary": "Область не установлена"},
	}
	before := FormatProfileEntries(entries)
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	var restored []map[string]any
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if after := FormatProfileEntries(restored); before != after {
		t.Fatal("JSON checkpoint changed scope/provenance rendering")
	}
	for _, want := range []string{`scope_id="session\n\"quoted\""`, `sources=["source:\"original\""]`, "scope unknown"} {
		if !strings.Contains(before, want) {
			t.Fatalf("missing literal metadata %q", want)
		}
	}
	if strings.Count(before, "\n") != 1 {
		t.Fatal("metadata injected a profile line")
	}
}
