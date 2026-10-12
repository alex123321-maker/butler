//go:build linux

package workspace

import (
	"os"
	"syscall"
)

func lockDirectory(dir *os.File) error {
	// Directory flock avoids an unlinkable/replaced lock-file identity. The
	// descriptor is close-on-exec and the kernel releases it on process death.
	return syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
