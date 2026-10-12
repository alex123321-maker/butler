package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceOwnerDiscardIndependentErasureObserver(t *testing.T) {
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil {
		t.Fatal("private root")
	}
	observer, err := NewObserver(root, DefaultLimits().MaxFileBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	id := strings.Repeat("a", 32)
	if os.Mkdir(filepath.Join(root, id), 0700) != nil {
		t.Fatal("copy")
	}
	if _, err = observer.Erased(context.Background(), id); err != ErrOutcomeUnknown {
		t.Fatal("present copy treated as erased")
	}
	if os.Remove(filepath.Join(root, id)) != nil {
		t.Fatal("remove fixture copy")
	}
	observation, err := observer.Erased(context.Background(), id)
	if err != nil || observation.Kind != "erased" || observation.WorkspaceID != id || len(observation.Digest) != 64 {
		t.Fatal("independent absence receipt")
	}
	outside := t.TempDir()
	if os.Symlink(outside, filepath.Join(root, id)) != nil {
		t.Skip("symlink support required")
	}
	if _, err = observer.Erased(context.Background(), id); err != ErrOutcomeUnknown {
		t.Fatal("symlink to empty namespace treated as absence")
	}
	if _, err = observer.Erased(context.Background(), "../"+id); err != ErrDenied {
		t.Fatal("caller host path accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = observer.Erased(ctx, id); err != ErrDenied {
		t.Fatal("cancelled receipt returned")
	}
}
