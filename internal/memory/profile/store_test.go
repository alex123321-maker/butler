package profile

import (
	"testing"
)

func TestNormalizeEntryDefaults(t *testing.T) {
	t.Parallel()
	entry, err := normalizeEntry(Entry{ScopeType: "session", ScopeID: "s-1", Key: "language"})
	if err != nil {
		t.Fatalf("normalizeEntry returned error: %v", err)
	}
	if entry.MemoryType != MemoryType {
		t.Fatalf("expected default memory type, got %q", entry.MemoryType)
	}
	if entry.Confidence != 1 {
		t.Fatalf("expected default confidence 1, got %v", entry.Confidence)
	}
	if entry.Status != StatusActive {
		t.Fatalf("expected default status active, got %q", entry.Status)
	}
}

func TestInheritScope(t *testing.T) {
	t.Parallel()
	previous := Entry{ScopeType: "session", ScopeID: "s-1", Key: "language"}
	entry := inheritScope(Entry{}, previous)
	if entry.ScopeType != previous.ScopeType || entry.ScopeID != previous.ScopeID || entry.Key != previous.Key {
		t.Fatalf("inheritScope returned %+v", entry)
	}
}
