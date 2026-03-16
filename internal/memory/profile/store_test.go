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
	if entry.ProvenanceJSON != `{"source_type":"","source_id":""}` {
		t.Fatalf("expected default provenance, got %q", entry.ProvenanceJSON)
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

func TestSaveRequiresStorePool(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).Save(nil, Entry{ScopeType: "session", ScopeID: "s-1", Key: "language"})
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}

func TestDefaultProvenanceJSON(t *testing.T) {
	t.Parallel()
	if got := defaultProvenanceJSON("run", "run-1"); got != `{"source_type":"run","source_id":"run-1"}` {
		t.Fatalf("unexpected provenance json %q", got)
	}
}
