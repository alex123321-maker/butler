package budget

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// ToolDeadlineTx reads an already reserved permanent effect in its caller TX.
// It neither spends again nor refunds a conflict. Missing/invalid reservations
// deny resource admission; absence of billing is not a budget allowance.
func (c *Controller) ToolDeadlineTx(ctx context.Context, tx pgx.Tx, ownerID, effectID string) (time.Time, error) {
	if c == nil || tx == nil || ownerID == "" || effectID == "" || c.Policy.ToolMillis <= 0 {
		return time.Time{}, ErrInvalid
	}
	var raw []byte
	var created, deadline, now time.Time
	err := tx.QueryRow(ctx, `SELECT r.request_json,r.created_at,LEAST(r.created_at+$3*INTERVAL '1 millisecond',MIN(s.deadline)),clock_timestamp()
 FROM execution_budget_reservations r CROSS JOIN LATERAL jsonb_array_elements_text(r.request_json->'scope_ids') AS ids(id)
 JOIN execution_budget_scopes s ON s.scope_id=ids.id AND s.owner_id=r.owner_id
 WHERE r.owner_id=$1 AND r.operation_id=$2 GROUP BY r.request_json,r.created_at
 HAVING COUNT(s.scope_id)=jsonb_array_length(r.request_json->'scope_ids')`, ownerID, "effect:"+effectID, c.Policy.ToolMillis).Scan(&raw, &created, &deadline, &now)
	if err != nil {
		return time.Time{}, err
	}
	var request Request
	if json.Unmarshal(raw, &request) != nil || request.OwnerID != ownerID || request.OperationID != "effect:"+effectID || !request.CostKnown || len(request.ScopeIDs) == 0 || request.Charge.validate() != nil || request.Charge.ExternalCalls != 1 || request.Charge.ActiveMillis != c.Policy.ToolMillis || !deadline.After(now) || created.After(now) {
		return time.Time{}, ErrInvalid
	}
	return deadline, nil
}
