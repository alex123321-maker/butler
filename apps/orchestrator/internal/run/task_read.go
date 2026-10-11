package run

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskSort defines list ordering for task-centric reads.
type TaskSort string

const (
	TaskSortStartedAtDesc TaskSort = "started_at_desc"
	TaskSortStartedAtAsc  TaskSort = "started_at_asc"
	TaskSortUpdatedAtDesc TaskSort = "updated_at_desc"
	TaskSortUpdatedAtAsc  TaskSort = "updated_at_asc"
)

// TaskListParams controls task list filtering and paging.
type TaskListParams struct {
	Status          string
	NeedsUserAction *bool
	WaitingReason   string
	SourceChannel   string
	Provider        string
	From            *time.Time
	To              *time.Time
	Query           string
	Limit           int
	Offset          int
	Sort            TaskSort
}

// TaskRow is a normalized read row for task-centric APIs.
type TaskRow struct {
	TaskStatus     string
	TaskRevision   int64
	Goal           string
	Legacy         bool
	ResultVerified bool
	TaskID         string
	RunID          string
	SessionKey     string
	SourceChannel  string
	SessionChannel string
	RunState       string
	StartedAt      time.Time
	UpdatedAt      time.Time
	FinishedAt     *time.Time
	ModelProvider  string
	AutonomyMode   string
	OutcomeSummary string
	ErrorSummary   string
	RiskLevel      string
}

// TaskReader exposes task-centric read model queries.
type TaskReader interface {
	ListTasks(ctx context.Context, params TaskListParams) ([]TaskRow, error)
}

func (r *PostgresRepository) ListTasks(ctx context.Context, params TaskListParams) ([]TaskRow, error) {
	query, args := buildTaskListQuery(params)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	items := make([]TaskRow, 0)
	for rows.Next() {
		var item TaskRow
		if err := rows.Scan(
			&item.TaskStatus, &item.TaskRevision, &item.Goal, &item.Legacy, &item.ResultVerified,
			&item.TaskID,
			&item.RunID,
			&item.SessionKey,
			&item.SourceChannel,
			&item.SessionChannel,
			&item.RunState,
			&item.StartedAt,
			&item.UpdatedAt,
			&item.FinishedAt,
			&item.ModelProvider,
			&item.AutonomyMode,
			&item.OutcomeSummary,
			&item.ErrorSummary,
			&item.RiskLevel,
		); err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return items, nil
}

func buildTaskListQuery(params TaskListParams) (string, []any) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	sortClause := "t.created_at DESC"
	switch params.Sort {
	case TaskSortStartedAtAsc:
		sortClause = "t.created_at ASC"
	case TaskSortUpdatedAtDesc:
		sortClause = "t.updated_at DESC"
	case TaskSortUpdatedAtAsc:
		sortClause = "t.updated_at ASC"
	}

	// Run activity/failure cannot establish consequences or likelihood of harm.
	// Typed action contracts and journal evidence belong to owner observations.
	riskLevelExpr := `'unknown'`

	query := strings.Builder{}
	query.WriteString(`
		SELECT
			t.status, t.revision, t.goal, t.legacy, (t.completion_result_id IS NOT NULL),
			t.task_id,
			COALESCE(r.run_id, ''),
			t.session_key,
			s.channel AS source_channel,
			s.channel AS session_channel,
			COALESCE(r.current_state, '') AS run_state,
			t.created_at,
			t.updated_at,
			t.finished_at,
			COALESCE(r.model_provider, ''),
			COALESCE(r.autonomy_mode, ''),
			COALESCE(completion_result.summary, last_assistant.content, '') AS outcome_summary,
			COALESCE(r.error_message, '') AS error_summary,
			` + riskLevelExpr + ` AS risk_level
		FROM tasks t
		INNER JOIN sessions s ON s.session_key = t.session_key
		LEFT JOIN runs r ON r.run_id = t.last_run_id
		LEFT JOIN task_results completion_result ON completion_result.result_id = t.completion_result_id
		LEFT JOIN LATERAL (
			SELECT m.content
			FROM messages m
			WHERE m.run_id = r.run_id AND m.role = 'assistant'
			ORDER BY m.created_at DESC
			LIMIT 1
		) AS last_assistant ON true
		WHERE 1=1
	`)

	args := make([]any, 0, 12)
	argIndex := 1
	add := func(clause string, value any) {
		query.WriteString(" AND ")
		query.WriteString(fmt.Sprintf(clause, argIndex))
		args = append(args, value)
		argIndex++
	}

	if value := strings.TrimSpace(strings.ToLower(params.Status)); value != "" {
		query.WriteString(" AND ")
		query.WriteString(statusFilterSQL(value))
	}
	if params.NeedsUserAction != nil {
		if *params.NeedsUserAction {
			query.WriteString(" AND (t.status = 'waiting_for_user' OR r.current_state = 'awaiting_approval')")
		} else {
			query.WriteString(" AND t.status <> 'waiting_for_user' AND COALESCE(r.current_state, '') <> 'awaiting_approval'")
		}
	}
	if value := strings.TrimSpace(strings.ToLower(params.WaitingReason)); value != "" {
		query.WriteString(" AND ")
		query.WriteString(waitingReasonFilterSQL(value))
	}
	if value := strings.TrimSpace(params.SourceChannel); value != "" {
		add("LOWER(s.channel) = $%d", strings.ToLower(value))
	}
	if value := strings.TrimSpace(params.Provider); value != "" {
		add("LOWER(r.model_provider) = $%d", strings.ToLower(value))
	}
	if params.From != nil {
		add("t.created_at >= $%d", params.From.UTC())
	}
	if params.To != nil {
		add("t.created_at <= $%d", params.To.UTC())
	}
	if value := strings.TrimSpace(params.Query); value != "" {
		search := "%" + value + "%"
		query.WriteString(fmt.Sprintf(" AND (t.task_id ILIKE $%d OR t.session_key ILIKE $%d OR t.goal ILIKE $%d OR COALESCE(last_assistant.content, '') ILIKE $%d OR COALESCE(r.error_message, '') ILIKE $%d)", argIndex, argIndex, argIndex, argIndex, argIndex))
		args = append(args, search)
		argIndex++
	}

	query.WriteString(" ORDER BY ")
	query.WriteString(sortClause)
	query.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1))
	args = append(args, params.Limit, params.Offset)

	return query.String(), args
}

func statusFilterSQL(status string) string {
	switch status {
	case "in_progress":
		return "t.status = 'active' AND COALESCE(r.current_state, '') <> 'awaiting_approval'"
	case "waiting_for_approval":
		return "((t.status = 'waiting_for_user' OR r.current_state = 'awaiting_approval') AND LOWER(s.channel) <> 'telegram')"
	case "waiting_for_reply_in_telegram":
		return "((t.status = 'waiting_for_user' OR r.current_state = 'awaiting_approval') AND LOWER(s.channel) = 'telegram')"
	case "completed":
		return "t.status = 'completed' AND t.completion_result_id IS NOT NULL"
	case "failed":
		return "t.status = 'blocked'"
	case "cancelled":
		return "t.status = 'cancelled'"
	case "completed_with_issues":
		return "t.status = 'blocked' AND r.current_state = 'timed_out'"
	case "active", "waiting_for_user", "blocked", "paused":
		return "t.status = '" + status + "'"
	default:
		return "1=0"
	}
}

func waitingReasonFilterSQL(reason string) string {
	switch reason {
	case "approval_required":
		return "r.current_state = 'awaiting_approval'"
	case "awaiting_model_resume":
		return "r.current_state = 'awaiting_model_resume'"
	case "none", "":
		return "r.current_state <> 'awaiting_approval' AND r.current_state <> 'awaiting_model_resume'"
	default:
		return "1=0"
	}
}
