package semantic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendTokenRequiresPrivateRegularBoundedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if e := os.WriteFile(path, []byte(strings.Repeat("fixture", 8)), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := NewAuthenticatedBackend(nil, path); e != nil {
		t.Fatal("private regular fixture denied")
	}
	link := filepath.Join(dir, "link")
	if e := os.Symlink(path, link); e == nil {
		if _, e = NewAuthenticatedBackend(nil, link); e == nil {
			t.Fatal("symlink token allowed")
		}
	}
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := NewAuthenticatedBackend(nil, path); e == nil {
		t.Fatal("public token allowed")
	}
	if e := os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(strings.Repeat("x", 4097)), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := NewAuthenticatedBackend(nil, path); e == nil {
		t.Fatal("oversize token allowed")
	}
}
