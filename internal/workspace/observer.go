package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/jackc/pgx/v5"
)

// Observer independently reads the operator's private filesystem. It returns
// content-free physical facts, not permission, acceptance or a model verdict.
// The caller must hold current authority/resource/workspace locks on tx.
type Observer struct {
	root         *os.Root
	maxFileBytes int64
}
type Observation struct {
	WorkspaceID string   `json:"workspace_id"`
	Kind        string   `json:"kind"`
	Snapshot    Snapshot `json:"snapshot"`
	Digest      string   `json:"digest"`
}

func NewObserver(rootPath string, maxFileBytes int64) (*Observer, error) {
	if maxFileBytes <= 0 || maxFileBytes > 64<<20 {
		return nil, ErrDenied
	}
	root, err := openPrivateRoot(rootPath)
	if err != nil {
		return nil, err
	}
	return &Observer{root, maxFileBytes}, nil
}
func (o *Observer) Close() error { return o.root.Close() }

// CheckOwner verifies this independently opened Observer is anchored to the
// same directory inode held by the runtime, including transient path swaps.
func (o *Observer) CheckOwner(owner *RootOwner) error {
	if o == nil {
		return ErrDenied
	}
	return owner.checkRoot(o.root)
}

// Check bounded directory entries without reading any unclassified content.
func exactEntries(r *os.Root, name string, names ...string) error {
	info, err := r.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return ErrDrift
	}
	dir, err := r.Open(name)
	if err != nil {
		return ErrDrift
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrDrift
	}
	entries, err := dir.ReadDir(len(names) + 1)
	if err != nil && !errors.Is(err, io.EOF) || len(entries) != len(names) {
		return ErrDrift
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.Name()] = true
	}
	for _, name := range names {
		if !seen[name] {
			return ErrDrift
		}
	}
	after, err := r.Lstat(name)
	if err != nil || !os.SameFile(info, after) || after.Mode().Perm() != 0700 {
		return ErrDrift
	}
	return nil
}

func privateCheckpoint(r *os.Root, expectedMarker string) error {
	if exactEntries(r, ".", "tree", "private") != nil || exactEntries(r, "private", "tmp", "write-pending") != nil || exactEntries(r, "private/tmp") != nil {
		return ErrDrift
	}
	treeInfo, treeErr := r.Lstat("tree")
	if treeErr != nil || !treeInfo.IsDir() || treeInfo.Mode()&os.ModeSymlink != 0 || treeInfo.Mode().Perm() != 0700 {
		return ErrDrift
	}
	info, err := r.Lstat("private/write-pending")
	if err != nil || info.Mode().Perm() != 0600 {
		return ErrDrift
	}
	marker, exists, err := physical(r, "private/write-pending", 32)
	decoded, decodeErr := hex.DecodeString(string(marker))
	if err != nil || !exists || decodeErr != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != string(marker) || expectedMarker != "" && string(marker) != expectedMarker {
		return ErrDrift
	}
	return nil
}

func (o *Observer) stableNamespace(id string, r *os.Root) error {
	info, err := o.root.Lstat(id)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return ErrDrift
	}
	dir, err := r.Open(".")
	if err != nil {
		return ErrDrift
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrDrift
	}
	return nil
}
func (o *Observer) namespace(ctx context.Context, tx pgx.Tx, a Access) (*os.Root, error) {
	if o == nil || tx == nil || !validRef(a.Ref) || ctx.Err() != nil {
		return nil, ErrDenied
	}
	b := a.Ref.Binding
	var valid bool
	if tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND state='active' FROM code_workspaces WHERE workspace_id=$1`, a.Ref.ID, b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision).Scan(&valid) != nil || !valid {
		return nil, ErrDenied
	}
	info, err := o.root.Lstat(a.Ref.ID)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, ErrDenied
	}
	r, err := o.root.OpenRoot(a.Ref.ID)
	if err != nil {
		return nil, ErrDenied
	}
	dir, err := r.Open(".")
	if err != nil {
		r.Close()
		return nil, ErrDenied
	}
	opened, err := dir.Stat()
	dir.Close()
	if err != nil || !os.SameFile(info, opened) {
		r.Close()
		return nil, ErrDrift
	}
	return r, nil
}
func observation(id, kind string, s Snapshot) Observation {
	raw, _ := json.Marshal([]any{id, kind, s})
	sum := sha256.Sum256(raw)
	return Observation{id, kind, s, hex.EncodeToString(sum[:])}
}
func (o *Observer) NamespaceTx(ctx context.Context, tx pgx.Tx, a Access) (Observation, error) {
	if a.Operation != "create" || a.Path != "" || a.Bytes != 0 || a.ContentSHA256 != "" {
		return Observation{}, ErrDenied
	}
	r, err := o.namespace(ctx, tx, a)
	if err != nil {
		return Observation{}, err
	}
	defer r.Close()
	if privateCheckpoint(r, a.Ref.ID) != nil || exactEntries(r, "tree") != nil || o.stableNamespace(a.Ref.ID, r) != nil || ctx.Err() != nil {
		return Observation{}, ErrDrift
	}
	return observation(a.Ref.ID, "empty_namespace", Snapshot{}), nil
}
func (o *Observer) FileTx(ctx context.Context, tx pgx.Tx, a Access) (Observation, error) {
	if o == nil || a.Operation != "write" || !validPath(a.Path) || !validDigest(a.ContentSHA256) || a.Bytes < 0 || a.Bytes > o.maxFileBytes {
		return Observation{}, ErrDenied
	}
	r, err := o.namespace(ctx, tx, a)
	if err != nil {
		return Observation{}, err
	}
	defer r.Close()
	if err = privateCheckpoint(r, ""); err != nil {
		return Observation{}, err
	}
	meta, err := snapshot(ctx, tx, a.Ref.ID, a.Path)
	if err != nil || meta.Version < 1 || meta.SHA256 != a.ContentSHA256 || meta.Bytes != a.Bytes {
		return Observation{}, ErrConflict
	}
	tree, err := r.OpenRoot("tree")
	if err != nil {
		return Observation{}, ErrDenied
	}
	defer tree.Close()
	if err = stableTree(r, tree); err != nil {
		return Observation{}, err
	}
	data, exists, err := physical(tree, a.Path, o.maxFileBytes)
	info, statErr := tree.Lstat(a.Path)
	if err != nil || !exists || !matches(meta, data, exists) || statErr != nil || info.Mode().Perm() != 0600 || stableTree(r, tree) != nil || o.stableNamespace(a.Ref.ID, r) != nil || ctx.Err() != nil {
		return Observation{}, ErrDrift
	}
	return observation(a.Ref.ID, "file_bytes", meta), nil
}
