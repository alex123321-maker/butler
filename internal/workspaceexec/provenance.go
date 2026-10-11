// Package workspaceexec owns workspace execution composition, not model policy
// or filesystem execution. Provenance is original DATA, never permission.
package workspaceexec

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = errors.New("current workspace source provenance required")

type Provenance struct {
	pool    *pgxpool.Pool
	sources *catalog.Store
}

// SourceContext is kernel-only prepared metadata. Its parent graph comes from
// actual run pins and immutable workspace history, never tool/model arguments.
type SourceContext struct {
	ref      workspace.Ref
	parents  []catalog.SourceRef
	previous int64
	run      string
}

// No owner actor or auxiliary job checker can cross into workspace source
// access. Retain only the actual worker token, deadline and cancellation.
func ownedContext(ctx context.Context) (context.Context, func(), error) {
	t, ok := ownership.TokenFromContext(ctx)
	_, owner := owneridentity.FromContext(ctx)
	if !ok || owner {
		return nil, nil, ErrDenied
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	base, cancel := context.WithDeadline(context.Background(), deadline)
	forward := context.AfterFunc(ctx, cancel)
	return ownership.WithToken(base, nil, t), func() { forward(); cancel() }, nil
}

func NewProvenance(pool *pgxpool.Pool, sources *catalog.Store) (*Provenance, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, ErrDenied
	}
	return &Provenance{pool, sources}, nil
}
func (p *Provenance) CheckReady(ctx context.Context) error {
	if p == nil || p.sources.CheckRestore(ctx) != nil {
		return ErrDenied
	}
	_, err := p.pool.Exec(ctx, `SELECT p.workspace_id,p.sequence,p.lease_id,p.source_id,p.observation_hash,d.workspace_id,d.status FROM code_workspace_sources p,code_workspace_deletions d WHERE FALSE`)
	return err
}

func worker(ctx context.Context, tx pgx.Tx, ref workspace.Ref) (resources.Binding, error) {
	t, ok := ownership.TokenFromContext(ctx)
	_, owner := owneridentity.FromContext(ctx)
	if !ok || owner || tx == nil || ref.ID == "" || ref.Binding.SessionKey != t.SessionKey {
		return resources.Binding{}, ErrDenied
	}
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT expires_at FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 FOR SHARE`, t.SessionKey, t.RunID, t.LeaseID, t.FenceEpoch).Scan(&expiry) != nil {
		return resources.Binding{}, ErrDenied
	}
	b := resources.Binding{OwnerID: ref.Binding.OwnerID, RunID: t.RunID, SessionKey: t.SessionKey, TaskID: ref.Binding.TaskID, TaskRevision: ref.Binding.TaskRevision, ConstraintsRevision: ref.Binding.ConstraintsRevision, LeaseEpoch: t.FenceEpoch, Deadline: expiry}
	if (resources.WorkerGuard{}).CheckTx(ctx, tx, b) != nil {
		return resources.Binding{}, ErrDenied
	}
	return b, nil
}

// PrepareTx must precede effect/budget/resource/workspace locks. A later check
// never discovers a different old source graph while holding those locks.
func (p *Provenance) PrepareTx(ctx context.Context, tx pgx.Tx, ref workspace.Ref, create bool) (SourceContext, error) {
	if p == nil {
		return SourceContext{}, ErrDenied
	}
	ctx, stop, contextErr := ownedContext(ctx)
	if contextErr != nil {
		return SourceContext{}, contextErr
	}
	defer stop()
	b, err := worker(ctx, tx, ref)
	if err != nil || p.sources.CheckRunSourcesTx(ctx, tx, b.RunID) != nil {
		return SourceContext{}, ErrDenied
	}
	c := SourceContext{ref: ref, run: b.RunID}
	var old catalog.SourceRef
	err = tx.QueryRow(ctx, `SELECT sequence,source_id,source_revision,access_revision,content_hash FROM code_workspace_sources WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1`, ref.ID).Scan(&c.previous, &old.ID, &old.Revision, &old.AccessRevision, &old.Hash)
	if create {
		if !errors.Is(err, pgx.ErrNoRows) {
			return SourceContext{}, ErrDenied
		}
	} else {
		if err != nil {
			return SourceContext{}, ErrDenied
		}
		c.parents = append(c.parents, old)
	}
	// Ancestors are inherited privacy dependencies. Request only graph roots,
	// so a child never gains selectable ancestor bodies from a run pin alone.
	rows, err := tx.Query(ctx, `SELECT p.source_id,p.source_revision,p.access_revision,p.content_hash FROM memory_run_sources p WHERE p.run_id=$1 AND NOT EXISTS(SELECT 1 FROM memory_source_dependencies d JOIN memory_run_sources child ON child.source_id=d.child_source_id AND child.run_id=p.run_id WHERE d.parent_source_id=p.source_id) ORDER BY p.source_id LIMIT 65`, b.RunID)
	if err != nil {
		return SourceContext{}, err
	}
	seen := map[string]bool{}
	for _, r := range c.parents {
		seen[r.ID] = true
	}
	for rows.Next() {
		var r catalog.SourceRef
		if err = rows.Scan(&r.ID, &r.Revision, &r.AccessRevision, &r.Hash); err != nil {
			rows.Close()
			return SourceContext{}, err
		}
		if !seen[r.ID] {
			c.parents = append(c.parents, r)
			seen[r.ID] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(c.parents) < 1 || len(c.parents) > 64 {
		return SourceContext{}, ErrDenied
	}
	if err = p.CheckTx(ctx, tx, c, ""); err != nil {
		return SourceContext{}, err
	}
	return c, nil
}

// CheckTx repeats the held source set and a fresh DB clock. After confirmation
// only this transaction's own invisible new source may extend that exact set.
func (p *Provenance) CheckTx(ctx context.Context, tx pgx.Tx, c SourceContext, lease string) error {
	if p == nil || tx == nil || c.run == "" || len(c.parents) < 1 || len(c.parents) > 64 {
		return ErrDenied
	}
	ctx, stop, contextErr := ownedContext(ctx)
	if contextErr != nil {
		return contextErr
	}
	defer stop()
	b, err := worker(ctx, tx, c.ref)
	if err != nil || b.RunID != c.run {
		return ErrDenied
	}
	if _, err = p.sources.AccessTx(ctx, tx, c.parents, "memory_read"); err != nil {
		return err
	}
	if err = p.sources.CheckRunSourcesTx(ctx, tx, c.run); err != nil {
		return err
	}
	var latest int64
	var sameTransaction bool
	err = tx.QueryRow(ctx, `SELECT sequence,lease_id=$2 AND xmin=pg_current_xact_id()::xid FROM code_workspace_sources WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1`, c.ref.ID, lease).Scan(&latest, &sameTransaction)
	if errors.Is(err, pgx.ErrNoRows) && c.previous == 0 {
		return nil
	}
	if err != nil || latest != c.previous && !(lease != "" && latest == c.previous+1 && sameTransaction) {
		return ErrDenied
	}
	if c.previous > 0 {
		var exact bool
		r := c.ref.Binding
		if tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND task_id=$4 AND task_revision=$5 AND constraints_revision=$6 AND state='active' FROM code_workspaces WHERE workspace_id=$1`, c.ref.ID, r.OwnerID, r.SessionKey, r.TaskID, r.TaskRevision, r.ConstraintsRevision).Scan(&exact) != nil || !exact {
			return ErrDenied
		}
	}
	return nil
}

// ConfirmTx is called only after the independent physical observer and under
// current source/resource/workspace locks. It publishes content-free task DATA
// with the complete original graph on the actual metadata transaction.
func (p *Provenance) ConfirmTx(ctx context.Context, tx pgx.Tx, c SourceContext, a workspace.Access, o workspace.Observation) (string, error) {
	if a.Ref != c.ref || a.Proof.RunID != c.run || a.Proof.LeaseID == "" || o.WorkspaceID != c.ref.ID || len(o.Digest) != 64 || a.Operation == "create" && o.Kind != "empty_namespace" || a.Operation == "write" && (o.Kind != "file_bytes" || o.Snapshot.Path != a.Path || o.Snapshot.SHA256 != a.ContentSHA256 || o.Snapshot.Bytes != a.Bytes) || a.Operation != "create" && a.Operation != "write" {
		return "", ErrDenied
	}
	if err := p.CheckTx(ctx, tx, c, ""); err != nil {
		return "", err
	}
	ctx, stop, contextErr := ownedContext(ctx)
	if contextErr != nil {
		return "", contextErr
	}
	defer stop()
	locator, _ := json.Marshal(map[string]any{"workspace_id": c.ref.ID, "sequence": c.previous + 1, "observation": o.Digest})
	src, err := p.sources.CreateTaskDerivedTx(ctx, tx, c.parents, "memory_read", "Prepared workspace provenance", "", locator)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO code_workspace_sources(workspace_id,sequence,lease_id,source_id,source_revision,access_revision,content_hash,observation_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.ref.ID, c.previous+1, a.Proof.LeaseID, src.ID, src.Revision, src.AccessRevision, src.Hash, o.Digest)
	if err != nil {
		return "", err
	}
	if err = p.CheckTx(ctx, tx, c, a.Proof.LeaseID); err != nil {
		return "", err
	}
	return src.ID, nil
}

// PurgeRuntime is a fixed operator filesystem executor, never a model callback.
type PurgeRuntime interface {
	PurgeWorkspace(context.Context, string) error
}

// PurgeDeleted performs only a committed source deletion's physical cleanup.
// Failures leave a durable pending job and permanently blocked workspace.
func (p *Provenance) PurgeDeleted(ctx context.Context, runtime PurgeRuntime) (bool, error) {
	if p == nil || runtime == nil {
		return false, ErrDenied
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	if err = p.sources.CheckRestoreTx(ctx, tx); err != nil {
		return false, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT d.workspace_id FROM code_workspace_deletions d JOIN memory_sources s ON s.source_id=d.source_id JOIN code_workspaces w ON w.workspace_id=d.workspace_id WHERE d.status='pending' AND s.status IN ('deleted','revoked') AND w.state='blocked' ORDER BY d.workspace_id LIMIT 1 FOR UPDATE OF d,w SKIP LOCKED`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = runtime.PurgeWorkspace(ctx, id); err != nil {
		return true, err
	}
	if _, err = tx.Exec(ctx, `UPDATE code_workspace_deletions SET status='done',completed_at=clock_timestamp() WHERE workspace_id=$1 AND status='pending'`, id); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
