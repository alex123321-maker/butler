//go:build linux

package workspace

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"syscall"
)

func directoryIdentity(info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrDenied
	}
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[:8], uint64(stat.Dev))
	binary.BigEndian.PutUint64(raw[8:], uint64(stat.Ino))
	digest := sha256.Sum256(raw[:])
	return hex.EncodeToString(digest[:]), nil
}

func lockDirectory(dir *os.File) error {
	// Directory flock avoids an unlinkable/replaced lock-file identity. The
	// descriptor is close-on-exec and the kernel releases it on process death.
	return syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
