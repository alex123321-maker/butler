//go:build !linux

package workspace

import "testing"

func TestWorkspaceRootOwnerUnsupportedPlatformDenies(t *testing.T) {
	if owner, err := LockRoot(t.TempDir()); err != ErrDenied {
		if owner != nil {
			_ = owner.Close()
		}
		t.Fatal("unsupported process ownership silently accepted")
	}
}
