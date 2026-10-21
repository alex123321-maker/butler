package budget

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// ModelAllowance is a selected adapter's declared conservative allowance. Its
// fields are supplied by control-plane code, never candidate/model content.
type ModelAllowance struct {
	Provider, Model           string
	InputTokens, OutputTokens int64
}

// CheckRootAdmissionTx lets the owning initiative service defer currently
// exhausted work before enqueue. It creates neither scopes nor reservations.
// Actual inference still charges atomically; capacity depleted after this
// transaction cannot authorize a provider request. Caller has checked current
// assignment and holds mandate/goal locks before taking budget locks.
func (c *Controller) CheckRootAdmissionTx(ctx context.Context, tx pgx.Tx, ownerID string, m MandateScope, a ModelAllowance) error {
	if m.ID == "" || ownerID == "" || m.OwnerID != ownerID || a.InputTokens <= 0 || a.OutputTokens <= 0 || a.OutputTokens > c.Policy.OutputTokens {
		return ErrInvalid
	}
	if m.Privacy != "cloud_allowed" {
		return &LoopError{Reason: "mandate_model_privacy"}
	}
	return c.checkRootAdmissionTx(ctx, tx, ownerID, &m, a)
}

// CheckScheduledRootAdmissionTx permits an exactly approved owner schedule with
// no broad mandate to use ordinary root/day bounds. It creates no reservation or
// permission; actual model/effect admission remains unchanged.
func (c *Controller) CheckScheduledRootAdmissionTx(ctx context.Context, tx pgx.Tx, ownerID string, m *MandateScope, a ModelAllowance) error {
	if c == nil || ownerID == "" || a.InputTokens <= 0 || a.OutputTokens <= 0 || a.OutputTokens > c.Policy.OutputTokens {
		return ErrInvalid
	}
	if m != nil {
		return c.CheckRootAdmissionTx(ctx, tx, ownerID, *m, a)
	}
	return c.checkRootAdmissionTx(ctx, tx, ownerID, nil, a)
}
func (c *Controller) checkRootAdmissionTx(ctx context.Context, tx pgx.Tx, ownerID string, m *MandateScope, a ModelAllowance) error {
	quote, err := c.Policy.ModelQuote(a.Provider, a.Model, a.InputTokens)
	if err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	// New root identity is not available until Task Service inserts it. Its finite
	// defaults can still deny this prospective first turn without creating a row.
	if _, err = addWithin(Scope{ScopeSpec: ScopeSpec{Kind: "task", OwnerID: ownerID, SubjectID: "prospective_root", Limits: c.Policy.Task, Deadline: now.Add(time.Duration(c.Policy.TaskLifetimeMillis) * time.Millisecond)}}, quote, now); err != nil {
		return err
	}
	var specs []ScopeSpec
	if !c.Policy.DisableOwnerDay {
		day := now.UTC().Truncate(24 * time.Hour)
		specs = append(specs, ScopeSpec{Kind: "owner_day", OwnerID: ownerID, SubjectID: day.Format("2006-01-02"), Limits: c.Policy.Day, Deadline: day.Add(24 * time.Hour)})
	}
	if m != nil {
		specs = append(specs, ScopeSpec{Kind: "mandate", OwnerID: ownerID, SubjectID: m.ID, Limits: m.Limits, Deadline: m.Deadline})
	}
	sort.Slice(specs, func(i, j int) bool {
		return ScopeID(specs[i].Kind, ownerID, specs[i].SubjectID) < ScopeID(specs[j].Kind, ownerID, specs[j].SubjectID)
	})
	for _, spec := range specs {
		if err = validateScope(spec); err != nil {
			return err
		}
		scope, readErr := scanScope(tx.QueryRow(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE scope_id=$1 FOR SHARE`, ScopeID(spec.Kind, ownerID, spec.SubjectID)))
		if errors.Is(readErr, ErrNotFound) {
			scope = Scope{ScopeSpec: spec}
		} else if readErr != nil {
			return readErr
		} else if scope.Limits != spec.Limits || !scope.Deadline.Equal(spec.Deadline.Truncate(time.Microsecond)) {
			return ErrConflict
		}
		if _, err = addWithin(scope, quote, now); err != nil {
			return err
		}
	}
	return nil
}
