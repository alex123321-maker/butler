package budget

import "context"

// ModelUsage reports durable attempts and conservative allowances, independently
// from task verification. Missing usage observations do not fabricate billed cost.
type ModelUsage struct {
	Attempts        int64  `json:"attempts"`
	InputAllowance  int64  `json:"input_allowance"`
	OutputAllowance int64  `json:"output_allowance"`
	ObservedInput   *int64 `json:"observed_input_tokens"`
	ObservedOutput  *int64 `json:"observed_output_tokens"`
}

func (l *Ledger) ModelRunUsage(ctx context.Context, owner, run string) (ModelUsage, error) {
	if owner == "" || run == "" {
		return ModelUsage{}, ErrInvalid
	}
	var u ModelUsage
	err := l.pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum(a.input_allowance),0),COALESCE(sum(a.output_allowance),0),CASE WHEN count(*)>0 AND count(a.actual_input_tokens)=count(*) THEN sum(a.actual_input_tokens) END,CASE WHEN count(*)>0 AND count(a.actual_output_tokens)=count(*) THEN sum(a.actual_output_tokens) END FROM execution_model_attempts a JOIN execution_budget_reservations r ON r.reservation_id=a.reservation_id WHERE r.owner_id=$1 AND a.run_id=$2`, owner, run).Scan(&u.Attempts, &u.InputAllowance, &u.OutputAllowance, &u.ObservedInput, &u.ObservedOutput)
	return u, err
}
