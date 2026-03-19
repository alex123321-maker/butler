package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TypeTaskReceived      = "task_received"
	TypeModelStarted      = "model_started"
	TypeApprovalRequested = "approval_requested"
	TypeApprovalResolved  = "approval_resolved"
	TypeTaskCompleted     = "task_completed"
	TypeTaskFailed        = "task_failed"

	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

type Record struct {
	ActivityID   int64
	RunID        string
	SessionKey   string
	ActivityType string
	Title        string
	Summary      string
	DetailsJSON  string
	ActorType    string
	Severity     string
	CreatedAt    time.Time
}

type CreateParams struct {
	RunID        string
	SessionKey   string
	ActivityType string
	Title        string
	Summary      string
	DetailsJSON  string
	ActorType    string
	Severity     string
	CreatedAt    time.Time
}

type ListParams struct {
	RunID      string
	SessionKey string
	Severity   string
	ActorType  string
	Limit      int
	Offset     int
	Since      *time.Time
	Until      *time.Time
}

type Repository interface {
	CreateActivity(ctx context.Context, params CreateParams) (Record, error)
	ListActivities(ctx context.Context, params ListParams) ([]Record, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateActivity(ctx context.Context, params CreateParams) (Record, error) {
	if params.CreatedAt.IsZero() {
		params.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(params.ActorType) == "" {
		params.ActorType = "system"
	}
	if strings.TrimSpace(params.Severity) == "" {
		params.Severity = SeverityInfo
	}
	if strings.TrimSpace(params.DetailsJSON) == "" {
		params.DetailsJSON = "{}"
	}
	const query = `
		INSERT INTO task_activity (
			run_id, session_key, activity_type, title, summary, details_json, actor_type, severity, created_at
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9)
		RETURNING activity_id, run_id, session_key, activity_type, title, summary, details_json::text, actor_type, severity, created_at
	`
	rec := Record{}
	err := r.pool.QueryRow(ctx, query,
		params.RunID,
		params.SessionKey,
		params.ActivityType,
		params.Title,
		params.Summary,
		params.DetailsJSON,
		params.ActorType,
		params.Severity,
		params.CreatedAt,
	).Scan(
		&rec.ActivityID,
		&rec.RunID,
		&rec.SessionKey,
		&rec.ActivityType,
		&rec.Title,
		&rec.Summary,
		&rec.DetailsJSON,
		&rec.ActorType,
		&rec.Severity,
		&rec.CreatedAt,
	)
	if err != nil {
		return Record{}, fmt.Errorf("create activity: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) ListActivities(ctx context.Context, params ListParams) ([]Record, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	const query = `
		SELECT activity_id, run_id, session_key, activity_type, title, summary, details_json::text, actor_type, severity, created_at
		FROM task_activity
		WHERE ($1 = '' OR run_id = $1)
		  AND ($2 = '' OR session_key = $2)
		  AND ($3 = '' OR severity = $3)
		  AND ($4 = '' OR actor_type = $4)
		  AND ($5::timestamptz IS NULL OR created_at >= $5)
		  AND ($6::timestamptz IS NULL OR created_at <= $6)
		ORDER BY created_at DESC, activity_id DESC
		LIMIT $7 OFFSET $8
	`
	rows, err := r.pool.Query(ctx, query,
		params.RunID,
		params.SessionKey,
		params.Severity,
		params.ActorType,
		params.Since,
		params.Until,
		params.Limit,
		params.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	items := make([]Record, 0)
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ActivityID, &rec.RunID, &rec.SessionKey, &rec.ActivityType, &rec.Title, &rec.Summary, &rec.DetailsJSON, &rec.ActorType, &rec.Severity, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan activity row: %w", err)
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activities: %w", err)
	}
	return items, nil
}
