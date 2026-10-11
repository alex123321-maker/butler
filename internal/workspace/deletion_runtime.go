package workspace

import (
	"context"
	"encoding/hex"
	"os"
)

// DeletionRuntime executes only kernel-queued privacy erasure. Its owning
// provenance worker validates the committed deletion; it grants no tool route.
type DeletionRuntime struct{ root *os.Root }

func NewDeletionRuntime(rootPath string) (*DeletionRuntime, error) {
	r, err := openPrivateRoot(rootPath)
	if err != nil {
		return nil, err
	}
	return &DeletionRuntime{r}, nil
}
func (r *DeletionRuntime) Close() error { return r.root.Close() }
func (r *DeletionRuntime) PurgeWorkspace(ctx context.Context, id string) error {
	decoded, err := hex.DecodeString(id)
	if r == nil || ctx.Err() != nil || err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != id {
		return ErrDenied
	}
	info, err := r.root.Lstat(id)
	if os.IsNotExist(err) {
		if syncDir(r.root, ".") != nil || ctx.Err() != nil {
			return ErrOutcomeUnknown
		}
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return ErrDenied
	}
	// Root.RemoveAll does not follow links. The fixed operator root contains only
	// private opaque namespaces; no model/queue field can select a host directory.
	if err = r.root.RemoveAll(id); err != nil {
		return ErrOutcomeUnknown
	}
	if syncDir(r.root, ".") != nil || ctx.Err() != nil {
		return ErrOutcomeUnknown
	}
	return nil
}
