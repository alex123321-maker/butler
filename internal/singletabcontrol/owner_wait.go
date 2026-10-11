package singletabcontrol

import (
	"context"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WaitingController accepts an owner decision on an existing finite request.
// It cannot request work, create a worker token, acquire resources or dispatch.
type WaitingController struct{ controller *Controller }

func NewWaiting(pool *pgxpool.Pool, metadata Metadata, sources resourceexec.SourceGuard, host string) (*WaitingController, error) {
	c, err := New(pool, metadata, waitingOwnerGuard{}, sources, host)
	if err != nil {
		return nil, err
	}
	return &WaitingController{controller: c}, nil
}

// The binding is read from immutable owning records, never channel arguments.
// LeaseEpoch=0 is deliberate: this is no execution proof, expired or otherwise.
func (w *WaitingController) binding(ctx context.Context, id string) (resources.Binding, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || w == nil || w.controller == nil || id == "" {
		return resources.Binding{}, ErrDenied
	}
	var b resources.Binding
	err := w.controller.pool.QueryRow(ctx, `SELECT owner_id,run_id,session_key,task_id,task_revision,constraints_revision,expires_at FROM guarded_single_tab_requests WHERE approval_id=$1 AND owner_id=$2 AND browser_instance=$3`, id, a.OwnerID, w.controller.host).Scan(&b.OwnerID, &b.RunID, &b.SessionKey, &b.TaskID, &b.TaskRevision, &b.ConstraintsRevision, &b.Deadline)
	if err != nil {
		return resources.Binding{}, ErrDenied
	}
	return b, nil
}

func (w *WaitingController) Select(ctx context.Context, id, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := w.binding(ctx, id)
	if err != nil {
		return "", err
	}
	return w.controller.Select(ctx, b, id, token)
}

func (w *WaitingController) Revoke(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := w.binding(ctx, id)
	if err != nil {
		return err
	}
	return w.controller.Revoke(ctx, b, id)
}

// Only this private owner-control composition uses this guard. Broker/runtime
// execution continues to require WorkerGuard and a fresh actual Session lease.
type waitingOwnerGuard struct{}

func (waitingOwnerGuard) CheckTx(ctx context.Context, tx pgx.Tx, b resources.Binding) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != b.OwnerID || b.LeaseEpoch != 0 || b.TaskID == "" || b.RunID == "" || b.TaskRevision < 1 || b.ConstraintsRevision < 1 || b.Deadline.IsZero() || (a.SessionKey != "" && a.SessionKey != b.SessionKey) {
		return ErrDenied
	}
	var valid bool
	// Controller.begin already holds the exclusive owning barrier. A resumed
	// live executor denies selection; an expired/released lease is never renewed.
	if tx.QueryRow(ctx, `SELECT (lease_id='' OR expires_at<=clock_timestamp()) AND $2::timestamptz>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR SHARE`, b.SessionKey, b.Deadline).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	var owner string
	if tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR SHARE`, b.SessionKey).Scan(&owner) != nil || owner != b.OwnerID {
		return ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT session_key=$2 AND task_id=$3 AND task_revision=$4 AND current_state='awaiting_approval' AND status NOT IN ('completed','failed','cancelled','timed_out') FROM runs WHERE run_id=$1 FOR SHARE`, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	if tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND revision=$4 AND constraints_revision=$5 AND last_run_id=$6 AND status='waiting_for_user' FROM tasks WHERE task_id=$1 FOR SHARE`, b.TaskID, b.OwnerID, b.SessionKey, b.TaskRevision, b.ConstraintsRevision, b.RunID).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !b.Deadline.After(at) {
		return ErrDenied
	}
	if _, err := actionauth.CheckTaskAuthorityTx(ctx, tx, b.TaskID, b.OwnerID, nil, at); err != nil {
		return ErrDenied
	}
	return nil
}
