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
}

func TestErrSnapshotNotFound(t *testing.T) {
	t.Parallel()

	if ErrSnapshotNotFound == nil {
		t.Fatal("expected not found sentinel error")
	}
}
