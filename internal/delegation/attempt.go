package delegation

import "time"

// AttemptReport is a fresh read-only observation, not a completed Report. There
// is no result ID, accomplished list, approval or semantic success verdict.
// Null usage stays unknown. Reserved counters include all branch reservations;
// the bounded attempt/action histories explicitly expose truncation.
type AttemptReport struct {
	TaskID              string          `json:"task_id"`
	ParentTaskID        string          `json:"parent_task_id"`
	TaskRevision        int64           `json:"task_revision"`
	ConstraintsRevision int64           `json:"constraints_revision"`
	ContractRevision    int64           `json:"contract_revision"`
	TaskStatus          string          `json:"task_status"`
	DelegationState     string          `json:"delegation_state"`
	ObservedAt          time.Time       `json:"observed_at"`
	Attempts            []FailedAttempt `json:"attempts"`
	AttemptsTruncated   bool            `json:"attempts_truncated"`
	Actions             []AttemptAction `json:"actual_actions"`
	ActionsTruncated    bool            `json:"actions_truncated"`
	ModelUsage          []AttemptUsage  `json:"model_usage"`
	ReservationIDs      []string        `json:"reservation_ids"`
	Reserved            Limits          `json:"reserved"`
	Sources             []SourceRef     `json:"source_refs"`
	Uncertainty         []Reason        `json:"uncertainty"`
}
type FailedAttempt struct {
	RunID          string `json:"run_id"`
	TaskRevision   int64  `json:"task_revision"`
	State          string `json:"state"`
	ErrorCode      string `json:"error_code"`
	DiagnosticCode string `json:"diagnostic_code"`
}
type AttemptAction struct {
	RunID        string `json:"run_id"`
	TaskRevision int64  `json:"task_revision"`
	ActionID     string `json:"action_id"`
	EffectID     string `json:"effect_id"`
	State        string `json:"state"`
}
type AttemptUsage struct {
	RunID         string `json:"run_id"`
	TaskRevision  int64  `json:"task_revision"`
	OperationID   string `json:"operation_id"`
	ReservationID string `json:"reservation_id"`
	InputTokens   *int64 `json:"input_tokens"`
	OutputTokens  *int64 `json:"output_tokens"`
}
