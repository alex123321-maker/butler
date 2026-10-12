package workspace

import (
	"os"
	"sync"
)

// RootOwner excludes other participating runtime processes from the same
// operator-owned directory. This is process lifetime ownership, not authority
// to execute a tool or evidence that an uncertain effect did not happen.
type RootOwner struct {
	mu   sync.Mutex
	root *os.Root
	dir  *os.File
}

func LockRoot(rootPath string) (*RootOwner, error) {
	root, err := openPrivateRoot(rootPath)
	if err != nil {
		return nil, ErrDenied
	}
	dir, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return nil, ErrDenied
	}
	if lockDirectory(dir) != nil {
		_ = dir.Close()
		_ = root.Close()
		return nil, ErrDenied
	}
	o := &RootOwner{root: root, dir: dir}
	if o.Check() != nil {
		_ = o.Close()
		return nil, ErrDenied
	}
	return o, nil
}

// Check binds ownership to both the held directory inode and its current
// operator path. A renamed/replaced/shared root cannot advertise readiness.
func (o *RootOwner) Check() error {
	if o == nil {
		return ErrDenied
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dir == nil {
		return ErrDenied
	}
	opened, err := o.dir.Stat()
	if err != nil {
		return ErrDenied
	}
	named, err := os.Lstat(o.root.Name())
	if err != nil || !named.IsDir() || named.Mode()&os.ModeSymlink != 0 || named.Mode().Perm()&0077 != 0 || opened.Mode().Perm()&0077 != 0 || !os.SameFile(opened, named) {
		return ErrDenied
	}
	return nil
}

// IdentityHash identifies the actual locked inode without exposing its path or
// device/inode numbers. Recovery compares it to the old physical executor's
// immutable binding; locking a replacement directory is not quiescence proof.
func (o *RootOwner) IdentityHash() (string, error) {
	if o == nil || o.Check() != nil {
		return "", ErrDenied
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dir == nil {
		return "", ErrDenied
	}
	info, err := o.dir.Stat()
	if err != nil {
		return "", ErrDenied
	}
	return directoryIdentity(info)
}

// checkRoot compares a component's already opened directory handle to the
// locked inode. Checking only its named path would miss a transient replacement
// restored before startup finishes, leaving that component anchored elsewhere.
func (o *RootOwner) checkRoot(root *os.Root) error {
	if o == nil || root == nil {
		return ErrDenied
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dir == nil {
		return ErrDenied
	}
	locked, err := o.dir.Stat()
	if err != nil || locked.Mode().Perm()&0077 != 0 {
		return ErrDenied
	}
	dir, err := root.Open(".")
	if err != nil {
		return ErrDenied
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !opened.IsDir() || opened.Mode().Perm()&0077 != 0 || !os.SameFile(locked, opened) {
		return ErrDenied
	}
	return nil
}

// Close releases the kernel lock only after the caller has drained all users.
// No lock file, PID, timeout, resource lease or effect history is modified.
func (o *RootOwner) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dir == nil {
		return nil
	}
	err := o.dir.Close()
	if e := o.root.Close(); err == nil {
		err = e
	}
	o.dir = nil
	if err != nil {
		return ErrDenied
	}
	return nil
}
