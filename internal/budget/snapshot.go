package budget

import "context"

type Attempt struct {
	OperationID     string `json:"operation_id"`
	RunID           string `json:"run_id"`
	InputAllowance  int64  `json:"input_allowance"`
	OutputAllowance int64  `json:"output_allowance"`
	ActualInput     *int64 `json:"actual_input_tokens"`
	ActualOutput    *int64 `json:"actual_output_tokens"`
}
type Snapshot struct {
	Scopes   []Scope   `json:"scopes"`
	Attempts []Attempt `json:"model_attempts"`
}

func (l *Ledger) SnapshotOwner(ctx context.Context, owner string) (Snapshot, error) {
	if owner == "" {
		return Snapshot{}, ErrInvalid
	}
	s := Snapshot{Scopes: []Scope{}, Attempts: []Attempt{}}
	rows, err := l.pool.Query(ctx, `SELECT `+scopeColumns+` FROM execution_budget_scopes WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 200`, owner)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		scope, e := scanScope(rows)
		if e != nil {
			rows.Close()
			return s, e
		}
		s.Scopes = append(s.Scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return s, err
	}
	rows, err = l.pool.Query(ctx, `SELECT a.operation_id,a.run_id,a.input_allowance,a.output_allowance,a.actual_input_tokens,a.actual_output_tokens FROM execution_model_attempts a JOIN execution_budget_reservations r ON r.reservation_id=a.reservation_id WHERE r.owner_id=$1 ORDER BY a.created_at DESC LIMIT 200`, owner)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Attempt
		if err = rows.Scan(&a.OperationID, &a.RunID, &a.InputAllowance, &a.OutputAllowance, &a.ActualInput, &a.ActualOutput); err != nil {
			return s, err
		}
		s.Attempts = append(s.Attempts, a)
	}
	return s, rows.Err()
}
