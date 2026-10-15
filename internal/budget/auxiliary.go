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
	// Quote is supplied by the trusted generative adapter after ModelQuote.
	// Nil preserves the existing local auxiliary contract.
	Quote          *Units
	PolicyID       string
	PolicyLimits   Units
	PolicyDeadline time.Time
	// AdditionalTasks are resolved only by the trusted cross-session job owner.
	// Every actual root/mandate shares the same atomic reservation and quote.
	AdditionalTasks []AuxiliaryTaskIdentity
}

type AuxiliaryTaskIdentity struct{ TaskID, MandateID string }

func (c *Controller) ReserveAuxiliaryTx(ctx context.Context, tx pgx.Tx, i AuxiliaryIdentity) (ModelAdmission, error) {
	if i.OwnerID == "" || i.JobID == "" || i.OperationID == "" || i.InputBytes < 1 || i.Tokens < 1 || i.ActiveMillis < 1 || len(i.AdditionalTasks) > 8 || len(i.AdditionalTasks) > 0 && i.TaskID != "" {
		return ModelAdmission{}, ErrInvalid
	}
	var now time.Time
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return ModelAdmission{}, e
	}
	var ids []string
	var specs []ScopeSpec
	if i.TaskID != "" {
		own, shared, e := c.scopeSpecsTx(ctx, tx, "", i.TaskID, i.MandateID)
		if e != nil {
			return ModelAdmission{}, e
		}
		if own != i.OwnerID {
			return ModelAdmission{}, ErrConflict
		}
		specs = shared
	} else {
		day := now.UTC().Truncate(24 * time.Hour)
		specs = append(specs, ScopeSpec{Kind: "owner_day", OwnerID: i.OwnerID, SubjectID: day.Format("2006-01-02"), Limits: c.Policy.Day, Deadline: day.Add(24 * time.Hour)})
	}
	for _, task := range i.AdditionalTasks {
		if task.TaskID == "" {
			return ModelAdmission{}, ErrInvalid
		}
		own, shared, err := c.scopeSpecsTx(ctx, tx, "", task.TaskID, task.MandateID)
		if err != nil {
			return ModelAdmission{}, err
		}
		if own != i.OwnerID {
			return ModelAdmission{}, ErrConflict
		}
		specs = append(specs, shared...)
	}
	specs = append(specs, ScopeSpec{Kind: "auxiliary", OwnerID: i.OwnerID, SubjectID: i.JobID, Limits: i.Limits, Deadline: i.Deadline})
	if i.PolicyID != "" {
		specs = append(specs, ScopeSpec{Kind: "auxiliary", OwnerID: i.OwnerID, SubjectID: "policy:" + i.PolicyID, Limits: i.PolicyLimits, Deadline: i.PolicyDeadline})
	}
	sort.Slice(specs, func(a, b int) bool {
		return ScopeID(specs[a].Kind, i.OwnerID, specs[a].SubjectID) < ScopeID(specs[b].Kind, i.OwnerID, specs[b].SubjectID)
	})
	unique := make([]ScopeSpec, 0, len(specs))
	for _, spec := range specs {
		if len(unique) > 0 {
			previous := unique[len(unique)-1]
			if ScopeID(previous.Kind, i.OwnerID, previous.SubjectID) == ScopeID(spec.Kind, i.OwnerID, spec.SubjectID) {
				if previous.OwnerID != spec.OwnerID || previous.Limits != spec.Limits || !previous.Deadline.Equal(spec.Deadline) {
					return ModelAdmission{}, ErrConflict
				}
				continue
			}
		}
		unique = append(unique, spec)
	}
	specs = unique
	for _, spec := range specs {
		s, e := c.Ledger.EnsureTx(ctx, tx, spec)
		if e != nil {
			return ModelAdmission{}, e
		}
		ids = append(ids, s.ID)
	}
	sort.Strings(ids)
	charge := Units{ModelTurns: 1, Tokens: i.Tokens, ActiveMillis: i.ActiveMillis, ProviderInputBytes: i.InputBytes}
	if i.Quote != nil {
		charge = *i.Quote
		if charge.validate() != nil || charge.ModelTurns != 1 || charge.Tokens != i.Tokens || charge.ActiveMillis != i.ActiveMillis || charge.ProviderInputBytes != i.InputBytes {
			return ModelAdmission{}, ErrInvalid
		}
	}
	r, _, e := c.Ledger.ReserveTx(ctx, tx, Request{OwnerID: i.OwnerID, OperationID: i.OperationID, ScopeIDs: ids, Charge: charge, CostKnown: true})
	if e != nil {
		return ModelAdmission{}, e
	}
	deadline := now.Add(time.Duration(i.ActiveMillis) * time.Millisecond)
	if i.Deadline.Before(deadline) {
		deadline = i.Deadline
	}
	return ModelAdmission{OperationID: i.OperationID, Deadline: deadline, ReservationID: r.ID}, nil
}
