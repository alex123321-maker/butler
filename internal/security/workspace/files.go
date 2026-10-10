// Package workspace bounds runtime file access to a controlled directory.
// The operator must exclude devices, procfs and attacker-controlled bind mounts.
package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrDenied = errors.New("workspace file access denied")

// ReadRegular uses the Root descriptor for traversal resistance, including
// symlink races. Only normalized relative paths and bounded regular files are
// admitted. Butler builders require a currently patched Go release.
func ReadRegular(rootPath, name string, limit int64) ([]byte, error) {
	if name == "" || filepath.IsAbs(name) || !filepath.IsLocal(name) || strings.HasSuffix(name, "/") || strings.HasSuffix(name, "\\") || strings.Contains(name, "\\") || filepath.Clean(name) != name || strings.ContainsRune(name, 0) || limit <= 0 {
		return nil, ErrDenied
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, ErrDenied
		}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, ErrDenied
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, ErrDenied
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrDenied
	}
	return data, nil
}
