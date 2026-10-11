package budget

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CheckTaskContinuationTx is a prospective next-turn check for an existing
// task. It joins the owner's transaction without creating/refilling scopes or
// charging a reservation. Actual provider admission still charges atomically.
func (c *Controller) CheckTaskContinuationTx(ctx context.Context, tx pgx.Tx, taskID, owner string, a ModelAllowance) error {
	if c == nil || taskID == "" || owner == "" || a.InputTokens <= 0 || a.OutputTokens != c.Policy.OutputTokens || c.resolveAuthority == nil {
		return ErrInvalid
	}
	quote, err := c.Policy.ModelQuote(a.Provider, a.Model, a.InputTokens)
	if err != nil {
		return err
	}
	derived, specs, err := c.scopeSpecsTx(context.WithValue(ctx, modelAdmissionKey{}, true), tx, "", taskID, "")
	if err != nil || derived != owner {
		return ErrConflict
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	for _, spec := range specs {
		scope, readErr := scanScope(tx.QueryRow(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE scope_id=$1 FOR SHARE`, ScopeID(spec.Kind, spec.OwnerID, spec.SubjectID)))
		if errors.Is(readErr, ErrNotFound) {
			scope = Scope{ScopeSpec: spec}
		} else if readErr != nil {
			return readErr
		} else if scope.Limits != spec.Limits || !scope.Deadline.Equal(spec.Deadline.Truncate(time.Microsecond)) {
			return ErrConflict
		}
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if _, err = addWithin(scope, quote, now); err != nil {
			return err
		}
	}
	// Repeat current authority time after budget waits, retaining all original limits.
	_, err = c.resolveAuthority(ctx, tx, taskID, owner)
	return err
}
