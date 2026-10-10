package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDeniesSiblingSymlinkAndNonregularFiles(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "allowed.txt"), []byte("allowed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("private-fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := ReadRegular(root, "allowed.txt", 32)
	if err != nil || string(value) != "allowed" {
		t.Fatal("allowed file denied")
	}
	for _, name := range []string{"../secret.txt", filepath.Join(other, "secret.txt"), "allowed.txt/", "./allowed.txt", ".", "allowed.txt\\"} {
		if _, err := ReadRegular(root, name, 32); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := os.Symlink(filepath.Join(other, "secret.txt"), filepath.Join(root, "escape")); err != nil {
		t.Skip("symlinks unavailable")
	}
	for _, name := range []string{"escape", "escape/"} {
		if _, err := ReadRegular(root, name, 32); err == nil {
			t.Fatal("symlink escaped workspace")
		}
	}
	if _, err := ReadRegular(root, "allowed.txt", 3); err == nil {
		t.Fatal("byte cap ignored")
	}
}
