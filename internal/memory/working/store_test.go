package working

import (
	"testing"
)

func TestSaveDefaults(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{SessionKey: "session-1"}
	if snapshot.EntitiesJSON != "" || snapshot.PendingStepsJSON != "" || snapshot.ScratchJSON != "" {
		t.Fatal("expected zero-value snapshot before normalization")
	}
	if snapshot.MemoryType != "" || snapshot.ProvenanceJSON != "" {
		t.Fatal("expected zero-value provenance fields before normalization")
	}
}

func TestErrSnapshotNotFound(t *testing.T) {
	t.Parallel()

	if ErrSnapshotNotFound == nil {
		t.Fatal("expected not found sentinel error")
	}
}

func TestSaveRequiresStorePool(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).Save(nil, Snapshot{SessionKey: "session-1"})
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}
