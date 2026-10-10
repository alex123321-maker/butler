package budget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Controller struct {
	pool             *pgxpool.Pool
	Ledger           *Ledger
	Policy           Policy
	resolveAuthority AuthorityResolver
}

// AuthorityResolver is installed by the owning task/mandate adapter. It performs
// current assignment checks on the caller transaction after domain locks, and
// returns only immutable scope policy, never credentials or model arguments.
type MandateScope struct {
	ID, OwnerID string
	Privacy     string
	Limits      Units
	Deadline    time.Time // immutable persisted allowance period
	// AuthorityDeadline is a transient release bound, never persisted as scope
	// policy or used to refill allowance after a goal/mandate revision.
	AuthorityDeadline time.Time
}
type AuthorityResolver func(context.Context, pgx.Tx, string, string) (MandateScope, error)

func (c *Controller) SetAuthorityResolver(resolve AuthorityResolver) { c.resolveAuthority = resolve }

func NewController(pool *pgxpool.Pool, p Policy) *Controller {
	return &Controller{pool: pool, Ledger: NewLedger(pool), Policy: p}
}
func NewOperationID() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "operation_" + hex.EncodeToString(b[:]), nil
}

// scopesTx resolves identity only through durable Butler records. Caller holds
// live authority/domain locks; this routine touches budget rows last.
func (c *Controller) scopesTx(ctx context.Context, tx pgx.Tx, runID, taskID, mandateID string) (string, []string, error) {
	var owner, session string
	if runID != "" {
		if err := tx.QueryRow(ctx, `SELECT s.user_id,r.session_key,COALESCE(r.task_id,'') FROM runs r JOIN sessions s ON s.session_key=r.session_key WHERE r.run_id=$1`, runID).Scan(&owner, &session, &taskID); err != nil {
			return "", nil, err
		}
	}
	var specs []ScopeSpec
	var assigned MandateScope
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return "", nil, err
	}
	day := now.UTC().Truncate(24 * time.Hour)
	if taskID != "" {
		var root string
		var created time.Time
		err := tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS(SELECT task_id,parent_task_id,owner_id,created_at,session_key FROM tasks WHERE task_id=$1 UNION ALL SELECT t.task_id,t.parent_task_id,t.owner_id,t.created_at,t.session_key FROM tasks t JOIN ancestors a ON t.task_id=a.parent_task_id) SELECT task_id,owner_id,created_at,session_key FROM ancestors WHERE parent_task_id IS NULL`, taskID).Scan(&root, &owner, &created, &session)
		if err != nil {
			return "", nil, err
		}
		specs = append(specs, ScopeSpec{Kind: "task", OwnerID: owner, SubjectID: root, Limits: c.Policy.Task, Deadline: created.Add(time.Duration(c.Policy.TaskLifetimeMillis) * time.Millisecond)})
		if c.resolveAuthority != nil {
			var err error
			assigned, err = c.resolveAuthority(ctx, tx, taskID, owner)
			if err != nil {
				return "", nil, err
			}
			if assigned.ID != "" {
				if assigned.OwnerID != owner {
					return "", nil, ErrConflict
				}
				mandateID = assigned.ID
				if inference, _ := ctx.Value(modelAdmissionKey{}).(bool); inference && assigned.Privacy != "cloud_allowed" {
					return "", nil, &LoopError{Reason: "mandate_model_privacy"}
				}
			}
		}
	} else {
		if owner == "" {
			return "", nil, ErrInvalid
		}
		specs = append(specs, ScopeSpec{Kind: "conversation", OwnerID: owner, SubjectID: session + ":" + day.Format("2006-01-02"), Limits: c.Policy.Conversation, Deadline: day.Add(24 * time.Hour)})
	}
	if mandateID != "" {
		if assigned.ID != "" {
			specs = append(specs, ScopeSpec{Kind: "mandate", OwnerID: owner, SubjectID: assigned.ID, Limits: assigned.Limits, Deadline: assigned.Deadline})
		} else {
			var mandateOwner string
			var expiry time.Time
			if err := tx.QueryRow(ctx, `SELECT owner_id,expires_at FROM action_mandates WHERE mandate_id=$1 AND revoked_at IS NULL FOR SHARE`, mandateID).Scan(&mandateOwner, &expiry); err != nil {
				return "", nil, err
			}
			if owner != mandateOwner {
				return "", nil, ErrConflict
			}
			specs = append(specs, ScopeSpec{Kind: "mandate", OwnerID: owner, SubjectID: mandateID, Limits: c.Policy.Mandate, Deadline: expiry})
		}
	}
	specs = append(specs, ScopeSpec{Kind: "owner_day", OwnerID: owner, SubjectID: day.Format("2006-01-02"), Limits: c.Policy.Day, Deadline: day.Add(24 * time.Hour)})
	// Consistent insert/lock ordering also matters when previously absent scopes
	// are first created by parallel model/tool/child admissions.
	sort.Slice(specs, func(i, j int) bool {
		return ScopeID(specs[i].Kind, owner, specs[i].SubjectID) < ScopeID(specs[j].Kind, owner, specs[j].SubjectID)
	})
	var ids []string
	for _, spec := range specs {
		s, err := c.Ledger.EnsureTx(ctx, tx, spec)
		if err != nil {
			return "", nil, err
		}
		ids = append(ids, s.ID)
	}
	return owner, ids, nil
}
func (c *Controller) ReserveToolTx(ctx context.Context, tx pgx.Tx, i ToolIdentity) error {
	if err := c.CheckToolTx(ctx, tx, i); err != nil {
		return err
	}
	owner, ids, err := c.scopesTx(ctx, tx, i.RunID, "", i.MandateID)
	if err != nil {
		return err
	}
	bytes := c.Policy.ToolEgressBytes
	if explicit, ok := ToolEgressAllowance(ctx); ok {
		if explicit < 0 || explicit > bytes {
			return ErrInvalid
		}
		bytes = explicit
	}
	_, _, err = c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: owner, OperationID: "effect:" + i.EffectID, ScopeIDs: ids, Charge: Units{ExternalCalls: 1, ActiveMillis: c.Policy.ToolMillis, EgressBytes: bytes}, CostKnown: true})
	return err
}
func (c *Controller) ReserveChildTx(ctx context.Context, tx pgx.Tx, parentID, mandateID, childID string) error {
	owner, ids, err := c.scopesTx(ctx, tx, "", parentID, mandateID)
	if err != nil {
		return err
	}
	_, _, err = c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: owner, OperationID: "child:" + childID, ScopeIDs: ids, Charge: Units{Children: 1}, CostKnown: true})
	if err == nil && c.resolveAuthority != nil {
		_, err = c.resolveAuthority(ctx, tx, parentID, owner)
	}
	return err
}

type ModelAdmission struct {
	OperationID   string
	Deadline      time.Time
	OutputTokens  int64
	ReservationID string
}

// Each invocation represents a new actual inference attempt, even after a
// restart. Its random operation is committed before transport is called.
func (c *Controller) AdmitModel(ctx context.Context, runID, mandateID, provider, model string, inputTokens int64) (ModelAdmission, error) {
	ctx = context.WithValue(ctx, modelAdmissionKey{}, true)
	charge, err := c.Policy.ModelQuote(provider, model, inputTokens)
	if err != nil {
		return ModelAdmission{}, err
	}
	op, err := NewOperationID()
	if err != nil {
		return ModelAdmission{}, err
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return ModelAdmission{}, fmt.Errorf("model admission requires trusted live ownership")
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return ModelAdmission{}, err
	}
	defer tx.Rollback(ctx)
	var live bool
	err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE run_id=$1 AND session_key=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, runID, token.SessionKey, token.LeaseID, token.FenceEpoch).Scan(&live)
	if err != nil {
		return ModelAdmission{}, fmt.Errorf("model authority expired")
	}
	var taskID, state, session string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),COALESCE(task_revision,0),current_state,session_key FROM runs WHERE run_id=$1 FOR SHARE`, runID).Scan(&taskID, &revision, &state, &session); err != nil {
		return ModelAdmission{}, err
	}
	if session != token.SessionKey || state == "completed" || state == "failed" || state == "cancelled" || state == "finalizing" {
		return ModelAdmission{}, ErrConflict
	}
	if taskID != "" {
		var current int64
		var status string
		if err = tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 FOR SHARE`, taskID).Scan(&current, &status); err != nil {
			return ModelAdmission{}, err
		}
		if current != revision || status != "active" {
			return ModelAdmission{}, ErrConflict
		}
	}
	owner, ids, err := c.scopesTx(ctx, tx, runID, "", mandateID)
	if err != nil {
		return ModelAdmission{}, err
	}
	r, _, err := c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: owner, OperationID: op, ScopeIDs: ids, Charge: charge, CostKnown: true})
	if err != nil {
		return ModelAdmission{}, err
	}
	var authorityDeadline time.Time
	if taskID != "" && c.resolveAuthority != nil {
		// Initial resolution already locked this assignment's authority rows.
		// Recheck time after budget waits without changing persisted scope limits.
		a, err := c.resolveAuthority(ctx, tx, taskID, owner)
		if err != nil {
			return ModelAdmission{}, err
		}
		authorityDeadline = a.AuthorityDeadline
	}
	if _, err = tx.Exec(ctx, `INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance) VALUES($1,$2,$3,$4,$5,$6,$7)`, op, runID, r.ID, provider, model, inputTokens, c.Policy.OutputTokens); err != nil {
		return ModelAdmission{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ModelAdmission{}, err
	}
	deadline := r.CreatedAt.Add(time.Duration(c.Policy.ModelMillis) * time.Millisecond)
	if !authorityDeadline.IsZero() && authorityDeadline.Before(deadline) {
		deadline = authorityDeadline
	}
	for _, id := range ids {
		scope, err := c.Ledger.Get(ctx, id)
		if err != nil {
			return ModelAdmission{}, err
		}
		if scope.Deadline.Before(deadline) {
			deadline = scope.Deadline
		}
	}
	return ModelAdmission{OperationID: op, ReservationID: r.ID, Deadline: deadline, OutputTokens: c.Policy.OutputTokens}, nil
}

type modelAdmissionKey struct{}

// ToolDeadline reads only owned committed budget rows after a successful claim.
// The runtime deadline is the earliest scope deadline or the reserved call time.
func (c *Controller) ToolDeadline(ctx context.Context, ownerID, effectID string) (time.Time, error) {
	var deadline time.Time
	err := c.pool.QueryRow(ctx, `SELECT LEAST(r.created_at + $3 * INTERVAL '1 millisecond',MIN(s.deadline)) FROM execution_budget_reservations r CROSS JOIN LATERAL jsonb_array_elements_text(r.request_json->'scope_ids') AS ids(id) JOIN execution_budget_scopes s ON s.scope_id=ids.id WHERE r.owner_id=$1 AND r.operation_id=$2 GROUP BY r.created_at`, ownerID, "effect:"+effectID, c.Policy.ToolMillis).Scan(&deadline)
	return deadline, err
}

// Observations never refund conservative reservations. Missing usage remains
// NULL, rather than being reported as zero actual tokens.
func (c *Controller) ObserveModel(ctx context.Context, op string, input, output *int64) error {
	if op == "" || input != nil && *input < 0 || output != nil && *output < 0 {
		return ErrInvalid
	}
	_, err := c.pool.Exec(ctx, `UPDATE execution_model_attempts SET actual_input_tokens=$2,actual_output_tokens=$3,observed_at=clock_timestamp() WHERE operation_id=$1 AND observed_at IS NULL`, op, input, output)
	return err
}
