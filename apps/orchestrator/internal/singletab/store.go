package singletab

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPendingApproval  = "PENDING_APPROVAL"
	StatusActive           = "ACTIVE"
	StatusTabClosed        = "TAB_CLOSED"
	StatusRevokedByUser    = "REVOKED_BY_USER"
	StatusExpired          = "EXPIRED"
	StatusHostDisconnected = "HOST_DISCONNECTED"
)

var (
	ErrSessionNotFound   = errors.New("single tab session not found")
	ErrActiveSessionBusy = errors.New("active single tab session already exists")
)

type Record struct {
	SingleTabSessionID string
	SessionKey         string
	CreatedByRunID     string
	ApprovalID         string
	Status             string
	BoundTabRef        string
	BrowserInstanceID  string
	HostID             string
	SelectedVia        string
	SelectedBy         string
	CurrentURL         string
	CurrentTitle       string
	StatusReason       string
	MetadataJSON       string
	CreatedAt          time.Time
	ActivatedAt        *time.Time
	LastSeenAt         *time.Time
	ReleasedAt         *time.Time
	ExpiresAt          *time.Time
	UpdatedAt          time.Time
}

type CreateParams struct {
	SingleTabSessionID string
	SessionKey         string
	CreatedByRunID     string
	ApprovalID         string
	Status             string
	BoundTabRef        string
	BrowserInstanceID  string
	HostID             string
	SelectedVia        string
	SelectedBy         string
	CurrentURL         string
	CurrentTitle       string
	StatusReason       string
	MetadataJSON       string
	CreatedAt          time.Time
	ActivatedAt        *time.Time
	ExpiresAt          *time.Time
}

type UpdateStatusParams struct {
	SingleTabSessionID string
	Status             string
	StatusReason       string
	SelectedVia        string
	SelectedBy         string
	CurrentURL         string
	CurrentTitle       string
	BrowserInstanceID  string
	HostID             string
	LastSeenAt         *time.Time
	ActivatedAt        *time.Time
	ReleasedAt         *time.Time
	ExpiresAt          *time.Time
	UpdatedAt          time.Time
}

type Repository interface {
	CreateSession(ctx context.Context, params CreateParams) (Record, error)
	GetSessionByID(ctx context.Context, sessionID string) (Record, error)
	GetSessionByApprovalID(ctx context.Context, approvalID string) (Record, error)
	GetActiveSessionBySessionKey(ctx context.Context, sessionKey string) (Record, error)
	UpdateSessionStatus(ctx context.Context, params UpdateStatusParams) (Record, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateSession(ctx context.Context, params CreateParams) (Record, error) {
	if params.CreatedAt.IsZero() {
		params.CreatedAt = time.Now().UTC()
	}
	if params.Status == "" {
		params.Status = StatusPendingApproval
	}
	if params.MetadataJSON == "" {
		params.MetadataJSON = "{}"
	}

	const query = `
		INSERT INTO single_tab_sessions (
			single_tab_session_id, session_key, created_by_run_id, approval_id, status, bound_tab_ref,
			browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json, created_at, activated_at, expires_at, updated_at
		) VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14::jsonb, $15, $16, $17, $15
		)
		RETURNING single_tab_session_id, session_key, created_by_run_id, COALESCE(approval_id, ''), status,
			bound_tab_ref, browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json::text, created_at, activated_at, last_seen_at, released_at, expires_at, updated_at
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query,
		params.SingleTabSessionID,
		params.SessionKey,
		params.CreatedByRunID,
		params.ApprovalID,
		params.Status,
		params.BoundTabRef,
		params.BrowserInstanceID,
		params.HostID,
		params.SelectedVia,
		params.SelectedBy,
		params.CurrentURL,
		params.CurrentTitle,
		params.StatusReason,
		params.MetadataJSON,
		params.CreatedAt,
		params.ActivatedAt,
		params.ExpiresAt,
	))
	if err != nil {
		if isUniqueViolation(err) && params.Status == StatusActive {
			return Record{}, ErrActiveSessionBusy
		}
		return Record{}, fmt.Errorf("create single tab session: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) GetSessionByID(ctx context.Context, sessionID string) (Record, error) {
	const query = `
		SELECT single_tab_session_id, session_key, created_by_run_id, COALESCE(approval_id, ''), status,
			bound_tab_ref, browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json::text, created_at, activated_at, last_seen_at, released_at, expires_at, updated_at
		FROM single_tab_sessions
		WHERE single_tab_session_id = $1
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query, sessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrSessionNotFound
		}
		return Record{}, fmt.Errorf("get single tab session: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) GetActiveSessionBySessionKey(ctx context.Context, sessionKey string) (Record, error) {
	const query = `
		SELECT single_tab_session_id, session_key, created_by_run_id, COALESCE(approval_id, ''), status,
			bound_tab_ref, browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json::text, created_at, activated_at, last_seen_at, released_at, expires_at, updated_at
		FROM single_tab_sessions
		WHERE session_key = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query, sessionKey, StatusActive))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrSessionNotFound
		}
		return Record{}, fmt.Errorf("get active single tab session: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) GetSessionByApprovalID(ctx context.Context, approvalID string) (Record, error) {
	const query = `
		SELECT single_tab_session_id, session_key, created_by_run_id, COALESCE(approval_id, ''), status,
			bound_tab_ref, browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json::text, created_at, activated_at, last_seen_at, released_at, expires_at, updated_at
		FROM single_tab_sessions
		WHERE approval_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query, approvalID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrSessionNotFound
		}
		return Record{}, fmt.Errorf("get single tab session by approval id: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) UpdateSessionStatus(ctx context.Context, params UpdateStatusParams) (Record, error) {
	if params.UpdatedAt.IsZero() {
		params.UpdatedAt = time.Now().UTC()
	}

	const query = `
		UPDATE single_tab_sessions
		SET status = $1,
			status_reason = $2,
			selected_via = $3,
			selected_by = $4,
			current_url = $5,
			current_title = $6,
			browser_instance_id = COALESCE(NULLIF($7, ''), browser_instance_id),
			host_id = COALESCE(NULLIF($8, ''), host_id),
			last_seen_at = COALESCE($9, last_seen_at),
			activated_at = COALESCE($10, activated_at),
			released_at = COALESCE($11, released_at),
			expires_at = COALESCE($12, expires_at),
			updated_at = $13
		WHERE single_tab_session_id = $14
		RETURNING single_tab_session_id, session_key, created_by_run_id, COALESCE(approval_id, ''), status,
			bound_tab_ref, browser_instance_id, host_id, selected_via, selected_by, current_url, current_title,
			status_reason, metadata_json::text, created_at, activated_at, last_seen_at, released_at, expires_at, updated_at
	`

	record, err := scanRecord(r.pool.QueryRow(ctx, query,
		params.Status,
		params.StatusReason,
		params.SelectedVia,
		params.SelectedBy,
		params.CurrentURL,
		params.CurrentTitle,
		params.BrowserInstanceID,
		params.HostID,
		params.LastSeenAt,
		params.ActivatedAt,
		params.ReleasedAt,
		params.ExpiresAt,
		params.UpdatedAt,
		params.SingleTabSessionID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrSessionNotFound
		}
		if isUniqueViolation(err) && params.Status == StatusActive {
			return Record{}, ErrActiveSessionBusy
		}
		return Record{}, fmt.Errorf("update single tab session status: %w", err)
	}
	return record, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner rowScanner) (Record, error) {
	var record Record
	err := scanner.Scan(
		&record.SingleTabSessionID,
		&record.SessionKey,
		&record.CreatedByRunID,
		&record.ApprovalID,
		&record.Status,
		&record.BoundTabRef,
		&record.BrowserInstanceID,
		&record.HostID,
		&record.SelectedVia,
		&record.SelectedBy,
		&record.CurrentURL,
		&record.CurrentTitle,
		&record.StatusReason,
		&record.MetadataJSON,
		&record.CreatedAt,
		&record.ActivatedAt,
		&record.LastSeenAt,
		&record.ReleasedAt,
		&record.ExpiresAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
