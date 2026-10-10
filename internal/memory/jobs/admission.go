package jobs

import (
	"context"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/security/content"
)

type Operation struct {
	ID, Kind, Endpoint, ServiceIdentity string
	InputBytes, Tokens                  int64
	Timeout                             time.Duration
	Origin                              content.Origin
}

// Admission reserves for each actual local attempt before IPC. Endpoint and
// service identity come from the configured transport adapter, not model input.
// Cloud and secret-derived auxiliary calls are deliberately unsupported.
func (s *Store) Admit(ctx context.Context, j Job, o Operation) (context.Context, func(), error) {
	if s.target == nil || s.budgets == nil || s.target.Identity == "" || o.ServiceIdentity != s.target.Identity || o.Endpoint != s.target.Endpoint || strings.Contains(o.Endpoint, "://") || !(strings.HasPrefix(o.Endpoint, "local-models:") || strings.HasPrefix(o.Endpoint, "unix:")) || o.Kind != j.Kind || o.ID == "" || o.Origin.Sensitivity == content.Secret || o.InputBytes < 1 || o.InputBytes > 8*1024*1024 || o.Tokens < 1 || o.Timeout <= 0 || o.Timeout > 2*time.Minute {
		return nil, nil, ErrClaim
	}
	if e := s.catalog.CheckRestore(ctx); e != nil {
		return nil, nil, e
	}
	jobCtx := s.Context(ctx, j)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, nil, e
	}
	defer tx.Rollback(ctx)
	sources, e := s.catalog.AccessTx(jobCtx, tx, j.Sources, j.Purpose)
	if e != nil {
		return nil, nil, e
	}
	for _, src := range sources {
		if src.Origin.Sensitivity == content.Secret {
			return nil, nil, ErrClaim
		}
	}
	actualID, e := budget.NewOperationID()
	if e != nil {
		return nil, nil, e
	}
	admission, e := s.budgets.ReserveAuxiliaryTx(jobCtx, tx, budget.AuxiliaryIdentity{OwnerID: j.OwnerID, TaskID: j.TaskID, MandateID: j.Authority.MandateID, JobID: j.ID, OperationID: "aux:" + j.ID + ":" + actualID, Limits: j.Allowance, Deadline: j.Deadline, InputBytes: o.InputBytes, Tokens: o.Tokens, ActiveMillis: o.Timeout.Milliseconds()})
	if e != nil {
		return nil, nil, e
	}
	// Recheck the DB clock after potentially waiting on shared allowance rows.
	checker := &claimChecker{s, j}
	if _, e = checker.CheckMemoryAccessTx(jobCtx, tx, j.Sources, j.Purpose); e != nil {
		return nil, nil, e
	}
	if e = checker.FinalizeMemoryAccessTx(jobCtx, tx, j.Sources, j.Purpose); e != nil {
		return nil, nil, e
	}
	deadline := admission.Deadline
	if j.ClaimUntil != nil && j.ClaimUntil.Before(deadline) {
		deadline = *j.ClaimUntil
	}
	if j.Authority.TaskID != "" {
		var now time.Time
		_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
		m, g, e := actionauth.CheckAssignmentTx(jobCtx, tx, j.Authority, now)
		if e != nil {
			return nil, nil, e
		}
		until, e := actionauth.AuthorityDeadline(m, g, now)
		if e != nil {
			return nil, nil, e
		}
		if until.Before(deadline) {
			deadline = until
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, nil, e
	}
	callCtx, cancel := context.WithDeadline(jobCtx, deadline)
	// Cancellation during an in-flight local request is prompt. Publication still
	// requires a fresh transactional proof, so a late callback cannot write.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-callCtx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(callCtx, 200*time.Millisecond)
				e := s.catalog.CheckObjects(checkCtx, nil, j.Purpose)
				stop()
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	return callCtx, func() { cancel(); <-done }, nil
}
