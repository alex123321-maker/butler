package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool   *pgxpool.Pool
	root   *os.Root
	guard  Guard
	limits Limits
}

// New requires an operator-provisioned private directory. No directory selected
// by a model, shared /tmp, repository root or credential mount is accepted here.
func New(pool *pgxpool.Pool, rootPath string, guard Guard, limits Limits) (*Store, error) {
	if pool == nil || guard == nil || !filepath.IsAbs(rootPath) || filepath.Clean(rootPath) != rootPath || limits.MaxFileBytes <= 0 || limits.MaxWorkspaceBytes < limits.MaxFileBytes || limits.MaxWorkspaceBytes > 64<<20 || limits.MaxFiles <= 0 || limits.MaxFiles > 1024 {
		return nil, ErrDenied
	}
	info, err := os.Lstat(rootPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrDenied
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, ErrDenied
	}
	dir, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrDenied
	}
	opened, err := dir.Stat()
	dir.Close()
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, ErrDenied
	}
	return &Store{pool, root, guard, limits}, nil
}
func (s *Store) Close() error { return s.root.Close() }

// Allocate is kernel-only namespace allocation, not admission. The kernel then
// acquires workspace:<ID> resource authority before Create. No host path leaks.
func Allocate(binding Binding) (Ref, error) {
	if !validBinding(binding) {
		return Ref{}, ErrDenied
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return Ref{}, err
	}
	return Ref{hex.EncodeToString(bytes[:]), binding}, nil
}
func validBinding(b Binding) bool {
	return b.OwnerID != "" && b.SessionKey != "" && b.TaskID != "" && b.TaskRevision > 0 && b.ConstraintsRevision > 0
}
func validRef(ref Ref) bool {
	value, err := hex.DecodeString(ref.ID)
	return err == nil && len(value) == 16 && hex.EncodeToString(value) == ref.ID && validBinding(ref.Binding)
}
func access(ref Ref, proof LeaseProof, operation, path string, bytes int64) (Access, error) {
	if !validRef(ref) || proof.LeaseID == "" || proof.RunID == "" || proof.FenceEpoch <= 0 || proof.ExpectedResourceVersion < 0 || proof.ResourceID != ResourceID(ref.ID) || proof.SessionKey != ref.Binding.SessionKey || proof.TaskID != ref.Binding.TaskID {
		return Access{}, ErrDenied
	}
	return Access{ref, proof, operation, path, bytes}, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) Create(ctx context.Context, ref Ref, proof LeaseProof) error {
	a, err := access(ref, proof, "create", "", 0)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return err
	}
	b := ref.Binding
	_, err = tx.Exec(ctx, `INSERT INTO code_workspaces(workspace_id,owner_id,session_key,task_id,task_revision,constraints_revision) VALUES($1,$2,$3,$4,$5,$6)`, ref.ID, b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision)
	if err != nil {
		return ErrDenied
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return err
	}
	if err = s.root.Mkdir(ref.ID, 0700); err != nil {
		return ErrDenied
	}
	r, err := s.root.OpenRoot(ref.ID)
	if err != nil {
		return ErrDenied
	}
	defer r.Close()
	for _, path := range []string{"tree", "private", "private/tmp"} {
		if err = r.Mkdir(path, 0700); err != nil {
			return ErrDenied
		}
	}
	if err = syncDir(r, "."); err != nil {
		return ErrDenied
	}
	if err = syncDir(s.root, "."); err != nil {
		return ErrDenied
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrOutcomeUnknown
	}
	return nil
}

func (s *Store) lock(ctx context.Context, tx pgx.Tx, ref Ref, write bool) (int64, error) {
	sql := `SELECT owner_id,session_key,task_id,task_revision,constraints_revision,state,generation FROM code_workspaces WHERE workspace_id=$1 FOR SHARE`
	if write {
		sql = `SELECT owner_id,session_key,task_id,task_revision,constraints_revision,state,generation FROM code_workspaces WHERE workspace_id=$1 FOR UPDATE`
	}
	var b Binding
	var state string
	var generation int64
	err := tx.QueryRow(ctx, sql, ref.ID).Scan(&b.OwnerID, &b.SessionKey, &b.TaskID, &b.TaskRevision, &b.ConstraintsRevision, &state, &generation)
	if err != nil || b != ref.Binding {
		return 0, ErrDenied
	}
	if state != "active" {
		return 0, ErrDrift
	}
	return generation, nil
}
func snapshot(ctx context.Context, tx pgx.Tx, id, name string) (Snapshot, error) {
	value := Snapshot{Path: name}
	err := tx.QueryRow(ctx, `SELECT version,content_sha256,byte_length FROM code_workspace_files WHERE workspace_id=$1 AND path=$2`, id, name).Scan(&value.Version, &value.SHA256, &value.Bytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, nil
	}
	return value, err
}
func (s *Store) tree(id string) (*os.Root, error) {
	info, err := s.root.Lstat(id)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrDenied
	}
	r, err := s.root.OpenRoot(id)
	if err != nil {
		return nil, ErrDenied
	}
	defer r.Close()
	info, err = r.Lstat("tree")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrDenied
	}
	return r.OpenRoot("tree")
}
func matches(value Snapshot, data []byte, exists bool) bool {
	return (value.Version == 0 && !exists) || (value.Version > 0 && exists && value.Bytes == int64(len(data)) && value.SHA256 == digest(data))
}

// Quarantine is monotonic denial, never recovery or permission. It is performed
// only after the original transaction releases its locks/connection.
func (s *Store) quarantine(ref Ref, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b := ref.Binding
	_, _ = s.pool.Exec(ctx, `UPDATE code_workspaces SET state='blocked',generation=generation+1,blocked_reason=$7 WHERE workspace_id=$1 AND owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND state='active'`, ref.ID, b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, reason)
}
func (s *Store) Read(ctx context.Context, ref Ref, proof LeaseProof, name string) (File, error) {
	if !validPath(name) {
		return File{}, ErrDenied
	}
	a, err := access(ref, proof, "read", name, s.limits.MaxFileBytes)
	if err != nil {
		return File{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return File{}, err
	}
	defer rollback(tx)
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return File{}, err
	}
	if _, err = s.lock(ctx, tx, ref, false); err != nil {
		return File{}, err
	}
	value, err := snapshot(ctx, tx, ref.ID, name)
	if err != nil {
		return File{}, err
	}
	r, err := s.tree(ref.ID)
	if err != nil {
		return File{}, err
	}
	defer r.Close()
	parent, err := s.root.OpenRoot(ref.ID)
	if err != nil {
		return File{}, ErrDenied
	}
	defer parent.Close()
	if pending(parent) {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return File{}, ErrDrift
	}
	data, exists, err := physical(r, name, s.limits.MaxFileBytes)
	if err != nil || !matches(value, data, exists) {
		rollback(tx)
		s.quarantine(ref, "physical_drift")
		if err != nil {
			return File{}, err
		}
		return File{}, ErrDrift
	}
	if value.Version == 0 {
		return File{}, os.ErrNotExist
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return File{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return File{}, err
	}
	return File{value, data}, nil
}

// Write is CAS within one workspace, including across Store instances/processes.
// Root metadata locks serialize all approved writers. The filesystem is not
// transactional with PostgreSQL: uncertain commit/drift blocks all later use.
func (s *Store) Write(ctx context.Context, ref Ref, proof LeaseProof, expected Snapshot, data []byte) (Snapshot, error) {
	name := expected.Path
	if !validPath(name) || int64(len(data)) > s.limits.MaxFileBytes {
		return Snapshot{}, ErrDenied
	}
	data = append([]byte(nil), data...)
	a, err := access(ref, proof, "write", name, int64(len(data)))
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer rollback(tx)
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return Snapshot{}, err
	}
	if _, err = s.lock(ctx, tx, ref, true); err != nil {
		return Snapshot{}, err
	}
	current, err := snapshot(ctx, tx, ref.ID, name)
	if err != nil {
		return Snapshot{}, err
	}
	if current != expected {
		return Snapshot{}, ErrConflict
	}
	var total, count int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(byte_length),0),count(*) FROM code_workspace_files WHERE workspace_id=$1`, ref.ID).Scan(&total, &count); err != nil {
		return Snapshot{}, err
	}
	if total-current.Bytes+int64(len(data)) > s.limits.MaxWorkspaceBytes || (current.Version == 0 && count >= int64(s.limits.MaxFiles)) {
		return Snapshot{}, ErrDenied
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return Snapshot{}, err
	}
	r, err := s.tree(ref.ID)
	if err != nil {
		return Snapshot{}, err
	}
	defer r.Close()
	parent, err := s.root.OpenRoot(ref.ID)
	if err != nil {
		return Snapshot{}, ErrDenied
	}
	defer parent.Close()
	if pending(parent) {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return Snapshot{}, ErrDrift
	}
	if err = checkedPath(r, name, true); err != nil {
		return Snapshot{}, err
	}
	old, exists, err := physical(r, name, s.limits.MaxFileBytes)
	if err != nil || !matches(current, old, exists) {
		rollback(tx)
		s.quarantine(ref, "physical_drift")
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{}, ErrDrift
	}
	if err = privateTemp(parent); err != nil {
		return Snapshot{}, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return Snapshot{}, err
	}
	tmp := "private/tmp/" + hex.EncodeToString(random[:])
	file, err := parent.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Snapshot{}, ErrDenied
	}
	defer parent.Remove(tmp)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Snapshot{}, ErrDenied
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		return Snapshot{}, err
	}
	old, exists, err = physical(r, name, s.limits.MaxFileBytes)
	if err != nil || !matches(current, old, exists) {
		rollback(tx)
		s.quarantine(ref, "physical_drift")
		return Snapshot{}, ErrDrift
	}
	if err = stableTree(parent, r); err != nil {
		rollback(tx)
		s.quarantine(ref, "physical_drift")
		return Snapshot{}, err
	}
	if err = markPending(parent, hex.EncodeToString(random[:])); err != nil {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return Snapshot{}, ErrOutcomeUnknown
	}
	if err = s.guard.CheckTx(ctx, tx, a); err != nil {
		_ = parent.Remove("private/write-pending")
		_ = syncDir(parent, "private")
		return Snapshot{}, err
	}
	if err = parent.Rename(tmp, "tree/"+name); err != nil {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return Snapshot{}, ErrOutcomeUnknown
	}
	if err = syncDir(r, filepath.Dir(name)); err != nil {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return Snapshot{}, ErrOutcomeUnknown
	}
	result := Snapshot{name, current.Version + 1, digest(data), int64(len(data))}
	_, err = tx.Exec(ctx, `INSERT INTO code_workspace_files(workspace_id,path,version,content_sha256,byte_length) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,path) DO UPDATE SET version=EXCLUDED.version,content_sha256=EXCLUDED.content_sha256,byte_length=EXCLUDED.byte_length,updated_at=clock_timestamp()`, ref.ID, name, result.Version, result.SHA256, result.Bytes)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE code_workspaces SET generation=generation+1 WHERE workspace_id=$1`, ref.ID)
	}
	if err == nil {
		err = s.guard.ConfirmWriteTx(ctx, tx, a, result)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		rollback(tx)
		s.quarantine(ref, "persistence_unknown")
		return Snapshot{}, ErrOutcomeUnknown
	}
	// At this point both content and metadata are committed. Failed marker cleanup
	// can block a later operation but cannot invent failure/undo of this write.
	_ = parent.Remove("private/write-pending")
	_ = syncDir(parent, "private")
	return result, nil
}
