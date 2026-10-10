package budget

import (
	"context"
	"strings"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// LocalTarget is composed by the operator with the internal service's fixed
// Docker-network address and pinned profiles. Pricing never proves locality.
// Its fields are private; inference requests cannot select an endpoint/model.
type LocalTarget struct{ endpoint, identity string }

func NewLocalTarget(endpoint, identity string) (LocalTarget, error) {
	if endpoint != "local-models:9090" || strings.TrimSpace(identity) == "" || len(identity) > 256 {
		return LocalTarget{}, ErrInvalid
	}
	return LocalTarget{endpoint, identity}, nil
}
func (t LocalTarget) Endpoint() string { return t.endpoint }
func (t LocalTarget) Identity() string { return t.identity }

type LocalOperation struct {
	Kind              string
	Items, InputBytes int64
	Timeout           time.Duration
	Origin            content.Origin
}

// LocalAccess is a trusted source-owner callback. It uses the caller transaction
// and locks catalog authority before allowance rows, never a second connection.
type LocalAccess func(context.Context, pgx.Tx) error

// CheckLocalAuthority validates a returned local result without charging a new
// attempt. Publication still uses its own current source transaction.
func (c *Controller) CheckLocalAuthority(ctx context.Context, runID string, access LocalAccess) error {
	if c == nil || c.pool == nil || access == nil {
		return ErrConflict
	}
	tx, e := c.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, _, _, e = c.localAuthorityTx(ctx, tx, runID); e != nil {
		return e
	}
	if e = access(ctx, tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (c *Controller) localQuote(target LocalTarget, o LocalOperation) (Units, error) {
	if c == nil || c.pool == nil || target.endpoint != "local-models:9090" || target.identity == "" || o.Origin.Sensitivity == content.Secret || o.Items < 1 || o.Items > 4 || o.InputBytes < 1 || o.InputBytes > 8192 || o.Timeout <= 0 || o.Timeout > time.Minute {
		return Units{}, ErrInvalid
	}
	model := "google/embeddinggemma-2"
	if o.Kind == "rerank" {
		model = "Qwen/Qwen3-Reranker-0.6B"
	} else if o.Kind != "query" && o.Kind != "index" {
		return Units{}, ErrInvalid
	}
	price, ok := c.Policy.Models["local:"+model]
	if !ok || price.Basis != "local_no_billing" || price.InputMicrosPerToken != 0 || price.OutputMicrosPerToken != 0 {
		return Units{}, ErrUnknownCost
	}
	// Tensor profiles enforce 2048 tokens per item. The extra allowance covers
	// the rerank query/template; this is a bound, not observed tokenizer usage.
	return Units{ModelTurns: 1, Tokens: 2048 * (o.Items + 1), ActiveMillis: o.Timeout.Milliseconds(), ProviderInputBytes: o.InputBytes}, nil
}

func (c *Controller) localAuthorityTx(ctx context.Context, tx pgx.Tx, runID string) (owner, task string, deadline time.Time, err error) {
	tok, ok := ownership.TokenFromContext(ctx)
	if !ok || tok.RunID != runID {
		err = ErrConflict
		return
	}
	var valid bool
	err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE run_id=$1 AND session_key=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, runID, tok.SessionKey, tok.LeaseID, tok.FenceEpoch).Scan(&valid)
	if err != nil {
		err = ErrConflict
		return
	}
	var revision int64
	var state, session string
	err = tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,''),COALESCE(r.task_revision,0),r.current_state,r.session_key,s.user_id FROM runs r JOIN sessions s ON s.session_key=r.session_key WHERE r.run_id=$1 FOR SHARE OF r`, runID).Scan(&task, &revision, &state, &session, &owner)
	if err != nil {
		return
	}
	if session != tok.SessionKey || state == "completed" || state == "failed" || state == "cancelled" || state == "finalizing" || state == "timed_out" {
		err = ErrConflict
		return
	}
	if task != "" {
		var current int64
		var status, taskOwner string
		err = tx.QueryRow(ctx, `SELECT revision,status,owner_id FROM tasks WHERE task_id=$1 FOR SHARE`, task).Scan(&current, &status, &taskOwner)
		if err != nil {
			return
		}
		if revision != current || status != "active" || taskOwner != owner || c.resolveAuthority == nil {
			err = ErrConflict
			return
		}
		a, e := c.resolveAuthority(ctx, tx, task, owner)
		if e != nil {
			err = e
			return
		}
		// Current mandate contract has local_only/cloud_allowed. Unknown policy
		// denies; cloud_allowed also allows the pinned internal local service.
		if a.ID != "" && a.Privacy != "local_only" && a.Privacy != "cloud_allowed" {
			err = ErrConflict
			return
		}
		deadline = a.AuthorityDeadline
	}
	return
}

// AdmitLocalModel reserves a fresh actual attempt across root/mandate/day/run
// scopes. A completed run lease is never resurrected for auxiliary work.
func (c *Controller) AdmitLocalModel(ctx context.Context, runID string, target LocalTarget, o LocalOperation, access LocalAccess) (context.Context, func(), error) {
	charge, e := c.localQuote(target, o)
	if e != nil {
		return nil, nil, e
	}
	if access == nil || o.Kind == "index" {
		return nil, nil, ErrConflict
	}
	op, e := NewOperationID()
	if e != nil {
		return nil, nil, e
	}
	tx, e := c.pool.Begin(ctx)
	if e != nil {
		return nil, nil, e
	}
	defer tx.Rollback(ctx)
	owner, _, authorityDeadline, e := c.localAuthorityTx(ctx, tx, runID)
	if e != nil {
		return nil, nil, e
	}
	if e = access(ctx, tx); e != nil {
		return nil, nil, e
	}
	actualOwner, ids, e := c.scopesTx(ctx, tx, runID, "", "")
	if e != nil {
		return nil, nil, e
	}
	if actualOwner != owner {
		return nil, nil, ErrConflict
	}
	var started time.Time
	if e = tx.QueryRow(ctx, `SELECT started_at FROM runs WHERE run_id=$1`, runID).Scan(&started); e != nil {
		return nil, nil, e
	}
	scope, e := c.Ledger.EnsureTx(ctx, tx, ScopeSpec{Kind: "auxiliary", OwnerID: owner, SubjectID: "live:" + runID, Limits: c.Policy.Conversation, Deadline: started.Add(time.Duration(c.Policy.TaskLifetimeMillis) * time.Millisecond)})
	if e != nil {
		return nil, nil, e
	}
	ids = append(ids, scope.ID)
	r, _, e := c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: owner, OperationID: "local:" + op, ScopeIDs: ids, Charge: charge, CostKnown: true})
	if e != nil {
		return nil, nil, e
	}
	// All domain rows are already held. Check current DB time after allowance
	// contention; an expired lease/hours window cannot reach IPC.
	_, _, authorityDeadline, e = c.localAuthorityTx(ctx, tx, runID)
	if e != nil {
		return nil, nil, e
	}
	if e = access(ctx, tx); e != nil {
		return nil, nil, e
	}
	deadline := r.CreatedAt.Add(o.Timeout)
	if !authorityDeadline.IsZero() && authorityDeadline.Before(deadline) {
		deadline = authorityDeadline
	}
	for _, id := range ids {
		var d time.Time
		if e = tx.QueryRow(ctx, `SELECT deadline FROM execution_budget_scopes WHERE scope_id=$1`, id).Scan(&d); e != nil {
			return nil, nil, e
		}
		if d.Before(deadline) {
			deadline = d
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, nil, e
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
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
				checkTx, err := c.pool.Begin(checkCtx)
				if err == nil {
					_, _, _, err = c.localAuthorityTx(checkCtx, checkTx, runID)
					if err == nil {
						err = access(checkCtx, checkTx)
					}
					_ = checkTx.Rollback(checkCtx)
				}
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return callCtx, func() { cancel(); <-done }, nil
}
