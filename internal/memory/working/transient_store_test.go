package working

import (
	"context"
	"testing"
	"time"
)

func TestTransientStateValidation(t *testing.T) {
	t.Parallel()

	if err := validateTransientState(TransientState{}, time.Minute); err == nil {
		t.Fatal("expected validation error for missing keys")
	}
	if err := validateTransientState(TransientState{SessionKey: "session-1", RunID: "run-1", ScratchJSON: `{"ok":true}`}, time.Minute); err != nil {
		t.Fatalf("expected valid transient state, got %v", err)
	}
	if err := validateTransientState(TransientState{SessionKey: "session-1", RunID: "run-1", ScratchJSON: `{bad}`}, time.Minute); err == nil {
		t.Fatal("expected invalid json error")
	}
}

func TestTransientKeyFormatting(t *testing.T) {
	t.Parallel()

	got := transientKey(" session ", " run ")
	want := "butler:memory:working:transient:session:run"
	if got != want {
		t.Fatalf("unexpected transient key %q want %q", got, want)
	}
}

func TestTransientStoreNilClientPanicsNotUsed(t *testing.T) {
	t.Parallel()

	store := NewTransientStore(nil)
	if store == nil {
		t.Fatal("expected transient store")
	}
	_ = context.Background()
}
