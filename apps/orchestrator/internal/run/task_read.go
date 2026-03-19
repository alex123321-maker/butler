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

	sortClause := "r.started_at DESC"
	switch params.Sort {
	case TaskSortStartedAtAsc:
		sortClause = "r.started_at ASC"
	case TaskSortUpdatedAtDesc:
		sortClause = "r.updated_at DESC"
	case TaskSortUpdatedAtAsc:
		sortClause = "r.updated_at ASC"
	}

	riskLevelExpr := `(
		CASE
			WHEN r.current_state IN ('failed','timed_out') THEN 'high'
			WHEN r.current_state IN ('awaiting_approval','tool_pending','tool_running') THEN 'medium'
			ELSE 'low'
		END
	)`

	query := strings.Builder{}
	query.WriteString(`
		SELECT
			r.run_id AS task_id,
			r.run_id,
			r.session_key,
			s.channel AS source_channel,
			s.channel AS session_channel,
			r.current_state AS run_state,
			r.started_at,
			r.updated_at,
			r.finished_at,
			r.model_provider,
			r.autonomy_mode,
			COALESCE(last_assistant.content, '') AS outcome_summary,
			COALESCE(r.error_message, '') AS error_summary,
			` + riskLevelExpr + ` AS risk_level
		FROM runs r
		INNER JOIN sessions s ON s.session_key = r.session_key
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
			query.WriteString(" AND r.current_state = 'awaiting_approval'")
		} else {
			query.WriteString(" AND r.current_state <> 'awaiting_approval'")
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
		add("r.started_at >= $%d", params.From.UTC())
	}
	if params.To != nil {
		add("r.started_at <= $%d", params.To.UTC())
	}
	if value := strings.TrimSpace(params.Query); value != "" {
		search := "%" + value + "%"
		query.WriteString(fmt.Sprintf(" AND (r.run_id ILIKE $%d OR r.session_key ILIKE $%d OR COALESCE(last_assistant.content, '') ILIKE $%d OR COALESCE(r.error_message, '') ILIKE $%d)", argIndex, argIndex, argIndex, argIndex))
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
		return "r.current_state IN ('created','queued','acquired','preparing','model_running','tool_pending','tool_running','awaiting_model_resume','finalizing')"
	case "waiting_for_approval":
		return "(r.current_state = 'awaiting_approval' AND LOWER(s.channel) <> 'telegram')"
	case "waiting_for_reply_in_telegram":
		return "(r.current_state = 'awaiting_approval' AND LOWER(s.channel) = 'telegram')"
	case "completed":
		return "r.current_state = 'completed'"
	case "failed":
		return "r.current_state = 'failed'"
	case "cancelled":
		return "r.current_state = 'cancelled'"
	case "completed_with_issues":
		return "r.current_state = 'timed_out'"
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
