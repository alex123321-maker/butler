package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
	"time"
)

func toolBudgetIdentity(g Grant, effectID string) budget.ToolIdentity {
	b := g.Binding
	return budget.ToolIdentity{RunID: b.RunID, MandateID: g.MandateID, EffectID: effectID, ToolName: b.ToolName, ArgsHash: b.ArgsHash, ContractHash: b.ContractHash, Effect: string(b.Effect), TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision}
}

// ResolveBudgetAuthority retains the current assignment locks on the caller's
// transaction. It does not acquire a second pool connection or reset policy on
// a mandate revision. Task/goal owners supply the immutable limit and deadline.
func ResolveBudgetAuthority(ctx context.Context, tx pgx.Tx, taskID, ownerID string) (budget.MandateScope, error) {
	a, err := CheckTaskAuthorityTx(ctx, tx, taskID, ownerID, nil, time.Now().UTC())
	if err != nil || a.TaskID == "" {
		return budget.MandateScope{}, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT mandate_data FROM action_mandates WHERE mandate_id=$1`, a.MandateID).Scan(&raw); err != nil {
		return budget.MandateScope{}, err
	}
	var m Mandate
	if err = json.Unmarshal(raw, &m); err != nil {
		return budget.MandateScope{}, err
	}
	b := m.Budget
	return budget.MandateScope{ID: a.MandateID, OwnerID: ownerID, Privacy: m.Privacy, Deadline: m.BudgetDeadline, Limits: budget.Units{ModelTurns: b.ModelTurns, Tokens: b.Tokens, CostMicros: b.CostMicros, ExternalCalls: b.ExternalCalls, Children: b.Children, ActiveMillis: b.ActiveMillis, EgressBytes: b.EgressBytes, ProviderInputBytes: b.ProviderInputBytes}}, nil
}
