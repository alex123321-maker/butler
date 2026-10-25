package actionauth

import (
	"context"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func toolBudgetIdentity(g Grant, effectID string) budget.ToolIdentity {
	b := g.Binding
	reply := false
	if b.ToolName == "communication.message" && b.Effect == EffectPrepare && len(g.InputClaimHash) == 64 && len(b.CommunicationDestinationsHash) == 64 && len(b.CredentialRefs) == 0 && b.Resource == "tool:communication.message" {
		hash, err := ContractHash(communicationcontrol.Contract(b.ToolName))
		reply = err == nil && hash == b.ContractHash
	}
	return budget.ToolIdentity{RunID: b.RunID, MandateID: g.MandateID, EffectID: effectID, ToolName: b.ToolName, OwnerReply: reply, OwnerCommunication: b.OwnerCommunication, OwnerCredentialInteraction: standardCredentialControl(b) && b.Effect == EffectPrepare && len(g.InputClaimHash) == 64, ArgsHash: b.ArgsHash, ContractHash: b.ContractHash, Effect: string(b.Effect), TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision}
}

// ResolveBudgetAuthority retains the current assignment locks on the caller's
// transaction. It does not acquire a second pool connection or reset policy on
// a mandate revision. Task/goal owners supply the immutable limit and deadline.
func ResolveBudgetAuthority(ctx context.Context, tx pgx.Tx, taskID, ownerID string) (budget.MandateScope, error) {
	var at time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return budget.MandateScope{}, err
	}
	a, err := CheckTaskAuthorityTx(ctx, tx, taskID, ownerID, nil, at)
	if err != nil || a.TaskID == "" {
		return budget.MandateScope{}, err
	}
	m, g, err := CheckAssignmentTx(ctx, tx, a, at)
	if err != nil {
		return budget.MandateScope{}, err
	}
	deadline, err := AuthorityDeadline(m, g, at)
	if err != nil {
		return budget.MandateScope{}, err
	}
	b := m.Budget
	return budget.MandateScope{ID: a.MandateID, OwnerID: ownerID, Privacy: m.Privacy, Deadline: m.BudgetDeadline, AuthorityDeadline: deadline, Limits: budget.Units{ModelTurns: b.ModelTurns, Tokens: b.Tokens, CostMicros: b.CostMicros, ExternalCalls: b.ExternalCalls, Children: b.Children, ActiveMillis: b.ActiveMillis, EgressBytes: b.EgressBytes, ProviderInputBytes: b.ProviderInputBytes}}, nil
}
