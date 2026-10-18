package semantic

import (
	"context"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

type LiveAdmission struct {
	budgets *budget.Controller
	guard   *CatalogGuard
	target  budget.LocalTarget
}

func NewLiveAdmission(b *budget.Controller, g *CatalogGuard, t budget.LocalTarget) (*LiveAdmission, error) {
	if b == nil || g == nil || t.Endpoint() != "local-models:9090" || t.Identity() == "" {
		return nil, ErrAdmission
	}
	return &LiveAdmission{b, g, t}, nil
}
func validateOperation(o Operation) error {
	if !o.Scope.valid() || o.ID == "" || o.Items < 1 || o.Items > 4 || o.InputBytes < 1 || o.InputBytes > 8192 || o.Timeout <= 0 || o.Timeout > time.Minute || o.Origin.Sensitivity == content.Secret {
		return ErrAdmission
	}
	if o.Kind == "query" {
		if len(o.ChunkIDs) != 0 || len(o.Sources) != 0 {
			return ErrAdmission
		}
	} else if o.Kind == "index" || o.Kind == "rerank" {
		if len(o.ChunkIDs) != o.Items || len(o.Sources) != o.Items {
			return ErrAdmission
		}
		for i, s := range o.Sources {
			if s.ID != o.ChunkIDs[i] || s.Scope != o.Scope {
				return ErrAdmission
			}
		}
	} else {
		return ErrAdmission
	}
	return nil
}
func (a *LiveAdmission) Admit(ctx context.Context, o Operation) (context.Context, func(), error) {
	if a == nil || validateOperation(o) != nil || o.Kind == "index" {
		return nil, nil, ErrAdmission
	}
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return nil, nil, ErrAdmission
	}
	access := func(ctx context.Context, tx pgx.Tx) error {
		return a.guard.checkStampsTx(ctx, tx, o.Sources)
	}
	return a.budgets.AdmitLocalModel(ctx, tok.RunID, a.target, budget.LocalOperation{Kind: o.Kind, Items: int64(o.Items), InputBytes: int64(o.InputBytes), Timeout: o.Timeout, Origin: o.Origin}, access)
}

func (a *LiveAdmission) CheckResult(ctx context.Context, o Operation) error {
	if a == nil || validateOperation(o) != nil {
		return ErrAdmission
	}
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok {
		return ErrAdmission
	}
	return a.budgets.CheckLocalAuthority(ctx, tok.RunID, func(ctx context.Context, tx pgx.Tx) error { return a.guard.checkStampsTx(ctx, tx, o.Sources) })
}

// JobAdmission is bound to one current durable claim and explicit operator
// pricing. Its Context is also used for reads/publications during Rebuild.
// Completed-task jobs remain denied by jobs.Store's current task contract.
type JobAdmission struct {
	store  *jobs.Store
	job    jobs.Job
	guard  *CatalogGuard
	target budget.LocalTarget
}

func NewJobAdmission(store *jobs.Store, j jobs.Job, g *CatalogGuard, target budget.LocalTarget, price budget.ModelPrice) (*JobAdmission, error) {
	if store == nil || g == nil || g.purpose != j.Purpose || j.Kind != "index" || j.ClaimID == "" || j.ClaimEpoch < 1 || target.Endpoint() != "local-models:9090" || target.Identity() == "" || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return nil, ErrAdmission
	}
	j.Sources = append([]catalog.SourceRef(nil), j.Sources...)
	return &JobAdmission{store, j, g, target}, nil
}
func (a *JobAdmission) Context(ctx context.Context) context.Context {
	return a.store.Context(ctx, a.job)
}
func (a *JobAdmission) Admit(ctx context.Context, o Operation) (context.Context, func(), error) {
	if a == nil || validateOperation(o) != nil || o.Kind != "index" {
		return nil, nil, ErrAdmission
	}
	ctx = a.Context(ctx)
	if e := a.guard.checkStamps(ctx, o.Sources); e != nil {
		return nil, nil, e
	}
	bounded, release, e := a.store.Admit(ctx, a.job, jobs.Operation{ID: o.ID, Kind: "index", Endpoint: a.target.Endpoint(), ServiceIdentity: a.target.Identity(), InputBytes: int64(o.InputBytes), Tokens: 2048 * int64(o.Items+1), Timeout: o.Timeout, Origin: o.Origin})
	if e != nil {
		return nil, nil, e
	}
	if e = a.guard.checkStamps(bounded, o.Sources); e != nil {
		release()
		return nil, nil, e
	}
	callCtx, cancel := context.WithCancel(bounded)
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
				err := a.observeStamps(checkCtx, o.Sources)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return callCtx, func() { cancel(); <-done; release() }, nil
}

// The periodic index observation is bound to this private durable job claim;
// unlike a live run, it cannot write memory_run_sources. Full catalog access,
// chunk bodies/revisions and final claim proof still run in the same transaction.
// Only the observation's unnecessary durable COMMIT is removed. Admission,
// CheckResult, source reads and publication retain their existing transactions.
func (a *JobAdmission) observeStamps(ctx context.Context, stamps []SourceStamp) error {
	ctx = a.Context(ctx)
	tx, err := a.guard.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = a.guard.checkStampsTx(ctx, tx, stamps); err != nil {
		return err
	}
	return tx.Rollback(ctx)
}

func (a *JobAdmission) CheckResult(ctx context.Context, o Operation) error {
	if a == nil || validateOperation(o) != nil {
		return ErrAdmission
	}
	return a.guard.checkStamps(a.Context(ctx), o.Sources)
}
