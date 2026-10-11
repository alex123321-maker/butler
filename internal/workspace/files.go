package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func validPath(name string) bool {
	if len(name) == 0 || len(name) > 512 || filepath.IsAbs(name) || !filepath.IsLocal(name) || filepath.Clean(name) != name || strings.ContainsAny(name, "\\\x00:") || strings.HasSuffix(name, "/") {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		part := strings.ToLower(component)
		if part == "" || part == "." || part == ".." || part == ".git" || part == ".ssh" || part == ".aws" || part == ".config" || part == ".docker" || part == ".kube" || part == "private" || part == "credentials" || part == "secrets" || part == "tmp" || part == ".tmp" || part == ".npmrc" || part == ".netrc" || part == "id_rsa" || part == "id_ed25519" || part == "id_ecdsa" || part == "authorized_keys" || part == "kubeconfig" || part == "token.json" || part == "tokens.json" || part == "cookies.json" || part == "storage_state.json" || strings.HasPrefix(part, ".env") || strings.HasPrefix(part, "credential") || strings.HasPrefix(part, "secrets.") || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") || strings.HasSuffix(part, ".p12") || strings.HasSuffix(part, ".pfx") {
			return false
		}
	}
	return true
}

// ValidCodePath shares the storage boundary with exact Broker/runtime argument
// validation. It grants no filesystem, workspace or source permission.
func ValidCodePath(name string) bool { return validPath(name) }
func digest(data []byte) string      { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }

func privateTemp(root *os.Root) error {
	for _, name := range []string{"private", "private/tmp"} {
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return ErrDenied
		}
	}
	return nil
}
func stableTree(parent, tree *os.Root) error {
	info, err := parent.Lstat("tree")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrDenied
	}
	dir, err := tree.Open(".")
	if err != nil {
		return ErrDenied
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrDrift
	}
	return nil
}
func syncDir(root *os.Root, name string) error {
	dir, err := root.Open(name)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// A private durable marker survives a crash/unknown PostgreSQL acknowledgement.
// It contains only an opaque attempt ID and no content, path or credential.
func pending(root *os.Root) bool {
	_, err := root.Lstat("private/write-pending")
	return !os.IsNotExist(err)
}
func markPending(root *os.Root, id string) error {
	file, err := root.OpenFile("private/write-pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrDrift
	}
	_, err = file.WriteString(id)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return ErrOutcomeUnknown
	}
	return syncDir(root, "private")
}

// Every component is checked in addition to Root traversal containment. Root is
// not a sandbox against mounts/devices; the operator owns the private directory.
func checkedPath(root *os.Root, path string, createParents bool) error {
	parts := strings.Split(path, "/")
	for i := range parts {
		name := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(name)
		if os.IsNotExist(err) && i < len(parts)-1 && createParents {
			if err = root.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
				return ErrDenied
			}
			info, err = root.Lstat(name)
		}
		if os.IsNotExist(err) && i == len(parts)-1 {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return ErrDenied
		}
		if i < len(parts)-1 && !info.IsDir() {
			return ErrDenied
		}
		if i == len(parts)-1 && (!info.Mode().IsRegular() || !singleLink(info)) {
			return ErrDenied
		}
	}
	return nil
}
func physical(root *os.Root, name string, limit int64) ([]byte, bool, error) {
	if err := checkedPath(root, name, false); err != nil {
		return nil, false, err
	}
	file, err := root.Open(name)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, ErrDenied
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !singleLink(info) || info.Size() > limit {
		return nil, false, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false, ErrDenied
	}
	after, err := root.Lstat(name)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) || !singleLink(after) {
		return nil, false, ErrDrift
	}
	return data, true, nil
}
