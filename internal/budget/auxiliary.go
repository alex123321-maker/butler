package budget

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuxiliaryIdentity is supplied by the durable job owner after checking its
// current claim, source hashes and authority on this transaction. It never
// borrows a completed run lease. Locality is checked independently by that owner.
type AuxiliaryIdentity struct {
	OwnerID, TaskID, MandateID, JobID, OperationID string
	Limits                                         Units
	Deadline                                       time.Time
	InputBytes, Tokens, ActiveMillis               int64
}

func (c *Controller) ReserveAuxiliaryTx(ctx context.Context, tx pgx.Tx, i AuxiliaryIdentity) (ModelAdmission, error) {
	if i.OwnerID == "" || i.JobID == "" || i.OperationID == "" || i.InputBytes < 1 || i.Tokens < 1 || i.ActiveMillis < 1 {
		return ModelAdmission{}, ErrInvalid
	}
	var now time.Time
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return ModelAdmission{}, e
	}
	var ids []string
	if i.TaskID != "" {
		own, shared, e := c.scopesTx(ctx, tx, "", i.TaskID, i.MandateID)
		if e != nil {
			return ModelAdmission{}, e
		}
		if own != i.OwnerID {
			return ModelAdmission{}, ErrConflict
		}
		ids = shared
	} else {
		day := now.UTC().Truncate(24 * time.Hour)
		s, e := c.Ledger.EnsureTx(ctx, tx, ScopeSpec{Kind: "owner_day", OwnerID: i.OwnerID, SubjectID: day.Format("2006-01-02"), Limits: c.Policy.Day, Deadline: day.Add(24 * time.Hour)})
		if e != nil {
			return ModelAdmission{}, e
		}
		ids = append(ids, s.ID)
	}
	scope, e := c.Ledger.EnsureTx(ctx, tx, ScopeSpec{Kind: "auxiliary", OwnerID: i.OwnerID, SubjectID: i.JobID, Limits: i.Limits, Deadline: i.Deadline})
	if e != nil {
		return ModelAdmission{}, e
	}
	ids = append(ids, scope.ID)
	sort.Strings(ids)
	r, _, e := c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: i.OwnerID, OperationID: i.OperationID, ScopeIDs: ids, Charge: Units{ModelTurns: 1, Tokens: i.Tokens, ActiveMillis: i.ActiveMillis, ProviderInputBytes: i.InputBytes}, CostKnown: true})
	if e != nil {
		return ModelAdmission{}, e
	}
	deadline := now.Add(time.Duration(i.ActiveMillis) * time.Millisecond)
	if i.Deadline.Before(deadline) {
		deadline = i.Deadline
	}
	return ModelAdmission{OperationID: i.OperationID, Deadline: deadline, ReservationID: r.ID}, nil
}
