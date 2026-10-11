//go:build unix

package workspace

import (
	"os"
	"syscall"
)

func singleLink(info os.FileInfo) bool {
	value, ok := info.Sys().(*syscall.Stat_t)
	return ok && value.Nlink == 1
}
