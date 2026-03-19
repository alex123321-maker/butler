package approval

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusExpired  = "expired"
	StatusFailed   = "failed"

	RequestedViaTelegram = "telegram"
	RequestedViaWeb      = "web"
	RequestedViaBoth     = "both"

	ResolvedViaTelegram = "telegram"
	ResolvedViaWeb      = "web"
	ResolvedViaSystem   = "system"
)

type Record struct {
	ApprovalID       string
	RunID            string
	SessionKey       string
	ToolCallID       string
	Status           string
	RequestedVia     string
	ResolvedVia      string
	ToolName         string
	ArgsJSON         string
	RiskLevel        string
	Summary          string
	DetailsJSON      string
	RequestedAt      time.Time
	ResolvedAt       *time.Time
	ResolvedBy       string
	ResolutionReason string
	ExpiresAt        *time.Time
	UpdatedAt        time.Time
}

type Event struct {
	EventID      int64
	ApprovalID   string
	RunID        string
	SessionKey   string
	EventType    string
	StatusBefore string
	StatusAfter  string
	ActorType    string
	ActorID      string
	Reason       string
	MetadataJSON string
	CreatedAt    time.Time
}

type CreateParams struct {
	ApprovalID   string
	RunID        string
	SessionKey   string
	ToolCallID   string
	RequestedVia string
	ToolName     string
	ArgsJSON     string
	RiskLevel    string
	Summary      string
	DetailsJSON  string
	ExpiresAt    *time.Time
	RequestedAt  time.Time
}

type ResolveParams struct {
	ApprovalID       string
	ExpectedStatus   string
	Status           string
	ResolvedVia      string
	ResolvedBy       string
	ResolutionReason string
	ResolvedAt       time.Time
}

var ErrApprovalNotFound = fmt.Errorf("approval not found")
var ErrApprovalStatusConflict = fmt.Errorf("approval status conflict")

type Repository interface {
	CreateApproval(ctx context.Context, params CreateParams) (Record, error)
	GetApprovalByToolCallID(ctx context.Context, toolCallID string) (Record, error)
	GetApprovalByID(ctx context.Context, approvalID string) (Record, error)
	ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]Record, error)
	ResolveApproval(ctx context.Context, params ResolveParams) (Record, error)
	InsertEvent(ctx context.Context, event Event) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateApproval(ctx context.Context, params CreateParams) (Record, error) {
	if params.RequestedAt.IsZero() {
		params.RequestedAt = time.Now().UTC()
	}
	if params.RequestedVia == "" {
		params.RequestedVia = RequestedViaTelegram
	}
	if params.RiskLevel == "" {
		params.RiskLevel = "medium"
	}
	if params.ArgsJSON == "" {
		params.ArgsJSON = "{}"
	}
	if params.DetailsJSON == "" {
		params.DetailsJSON = "{}"
	}

	const query = `
		INSERT INTO approvals (
			approval_id, run_id, session_key, tool_call_id, status, requested_via,
			tool_name, args_json, risk_level, summary, details_json,
			requested_at, expires_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11::jsonb, $12, $13, $12)
		ON CONFLICT (approval_id) DO NOTHING
		RETURNING approval_id, run_id, session_key, tool_call_id, status, requested_via, COALESCE(resolved_via, ''), tool_name,
			args_json::text, risk_level, summary, details_json::text, requested_at, resolved_at, resolved_by, resolution_reason, expires_at, updated_at
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query,
		params.ApprovalID,
		params.RunID,
		params.SessionKey,
		params.ToolCallID,
		StatusPending,
		params.RequestedVia,
		params.ToolName,
		params.ArgsJSON,
		params.RiskLevel,
		params.Summary,
		params.DetailsJSON,
		params.RequestedAt,
		params.ExpiresAt,
	))
	if err == nil {
		return record, nil
	}
	if err != pgx.ErrNoRows {
		return Record{}, fmt.Errorf("create approval: %w", err)
	}

	existing, getErr := r.GetApprovalByID(ctx, params.ApprovalID)
	if getErr != nil {
		return Record{}, getErr
	}
	return existing, nil
}

func (r *PostgresRepository) GetApprovalByToolCallID(ctx context.Context, toolCallID string) (Record, error) {
	const query = `
		SELECT approval_id, run_id, session_key, tool_call_id, status, requested_via, COALESCE(resolved_via, ''), tool_name,
			args_json::text, risk_level, summary, details_json::text, requested_at, resolved_at, resolved_by, resolution_reason, expires_at, updated_at
		FROM approvals
		WHERE tool_call_id = $1
		ORDER BY requested_at DESC
		LIMIT 1
	`
	rec, err := scanRecord(r.pool.QueryRow(ctx, query, toolCallID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return Record{}, ErrApprovalNotFound
		}
		return Record{}, fmt.Errorf("get approval by tool_call_id: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) GetApprovalByID(ctx context.Context, approvalID string) (Record, error) {
	const query = `
		SELECT approval_id, run_id, session_key, tool_call_id, status, requested_via, COALESCE(resolved_via, ''), tool_name,
			args_json::text, risk_level, summary, details_json::text, requested_at, resolved_at, resolved_by, resolution_reason, expires_at, updated_at
		FROM approvals
		WHERE approval_id = $1
	`
	rec, err := scanRecord(r.pool.QueryRow(ctx, query, approvalID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return Record{}, ErrApprovalNotFound
		}
		return Record{}, fmt.Errorf("get approval by id: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]Record, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		SELECT approval_id, run_id, session_key, tool_call_id, status, requested_via, COALESCE(resolved_via, ''), tool_name,
			args_json::text, risk_level, summary, details_json::text, requested_at, resolved_at, resolved_by, resolution_reason, expires_at, updated_at
		FROM approvals
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR run_id = $2)
		  AND ($3 = '' OR session_key = $3)
		ORDER BY requested_at DESC, approval_id DESC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.pool.Query(ctx, query, status, runID, sessionKey, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()

	items := make([]Record, 0)
	for rows.Next() {
		rec, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan approval row: %w", scanErr)
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approvals: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) ResolveApproval(ctx context.Context, params ResolveParams) (Record, error) {
	if params.ResolvedAt.IsZero() {
		params.ResolvedAt = time.Now().UTC()
	}
	if params.ExpectedStatus == "" {
		params.ExpectedStatus = StatusPending
	}
	const query = `
		UPDATE approvals
		SET status = $3,
			resolved_via = $4,
			resolved_by = $5,
			resolution_reason = $6,
			resolved_at = $7,
			updated_at = $7
		WHERE approval_id = $1 AND status = $2
		RETURNING approval_id, run_id, session_key, tool_call_id, status, requested_via, COALESCE(resolved_via, ''), tool_name,
			args_json::text, risk_level, summary, details_json::text, requested_at, resolved_at, resolved_by, resolution_reason, expires_at, updated_at
	`
	rec, err := scanRecord(r.pool.QueryRow(ctx, query,
		params.ApprovalID,
		params.ExpectedStatus,
		params.Status,
		params.ResolvedVia,
		params.ResolvedBy,
		params.ResolutionReason,
		params.ResolvedAt,
	))
	if err == nil {
		return rec, nil
	}
	if err == pgx.ErrNoRows {
		existing, getErr := r.GetApprovalByID(ctx, params.ApprovalID)
		if getErr != nil {
			if getErr == ErrApprovalNotFound {
				return Record{}, ErrApprovalNotFound
			}
			return Record{}, getErr
		}
		if existing.Status != params.ExpectedStatus {
			return Record{}, ErrApprovalStatusConflict
		}
		return Record{}, ErrApprovalNotFound
	}
	return Record{}, fmt.Errorf("resolve approval: %w", err)
}

func (r *PostgresRepository) InsertEvent(ctx context.Context, event Event) error {
	if event.MetadataJSON == "" {
		event.MetadataJSON = "{}"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	const query = `
		INSERT INTO approval_events (
			approval_id, run_id, session_key, event_type, status_before, status_after,
			actor_type, actor_id, reason, metadata_json, created_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, $9, $10::jsonb, $11)
	`
	if _, err := r.pool.Exec(ctx, query,
		event.ApprovalID,
		event.RunID,
		event.SessionKey,
		event.EventType,
		event.StatusBefore,
		event.StatusAfter,
		event.ActorType,
		event.ActorID,
		event.Reason,
		event.MetadataJSON,
		event.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert approval event: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner rowScanner) (Record, error) {
	var rec Record
	err := scanner.Scan(
		&rec.ApprovalID,
		&rec.RunID,
		&rec.SessionKey,
		&rec.ToolCallID,
		&rec.Status,
		&rec.RequestedVia,
		&rec.ResolvedVia,
		&rec.ToolName,
		&rec.ArgsJSON,
		&rec.RiskLevel,
		&rec.Summary,
		&rec.DetailsJSON,
		&rec.RequestedAt,
		&rec.ResolvedAt,
		&rec.ResolvedBy,
		&rec.ResolutionReason,
		&rec.ExpiresAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}
