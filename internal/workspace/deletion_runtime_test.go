package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeletionRuntimeContainsPrivateErasure(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := NewDeletionRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	id := "0123456789abcdef0123456789abcdef"
	other := "fedcba9876543210fedcba9876543210"
	for _, namespace := range []string{id, other} {
		if err = os.Mkdir(filepath.Join(root, namespace), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, namespace, "fixture"), []byte("unrelated private fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{".", "..", root, "../" + id, "private", id + "/tree", "0123456789ABCDEF0123456789ABCDEF"} {
		if err = r.PurgeWorkspace(context.Background(), invalid); err == nil {
			t.Fatal("invalid erasure path admitted")
		}
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err = r.PurgeWorkspace(cancelled, id); err == nil {
		t.Fatal("cancelled erasure ran")
	}
	if err = r.PurgeWorkspace(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err = r.PurgeWorkspace(context.Background(), id); err != nil {
		t.Fatal("idempotent erase", err)
	}
	if _, err = os.Lstat(filepath.Join(root, id)); !os.IsNotExist(err) {
		t.Fatal("selected namespace retained", err)
	}
	if body, err := os.ReadFile(filepath.Join(root, other, "fixture")); err != nil || string(body) != "unrelated private fixture" {
		t.Fatal("other copy erased", err)
	}
	if err = os.Symlink(filepath.Join(root, other), filepath.Join(root, id)); err != nil {
		t.Fatal(err)
	}
	if err = r.PurgeWorkspace(context.Background(), id); err == nil {
		t.Fatal("namespace symlink erased")
	}
	if _, err = os.Stat(filepath.Join(root, other, "fixture")); err != nil {
		t.Fatal("linked other copy erased", err)
	}
}
