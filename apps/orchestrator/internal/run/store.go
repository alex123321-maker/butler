package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRunNotFound     = fmt.Errorf("run not found")
	ErrSessionNotFound = fmt.Errorf("session not found")
	ErrRunDuplicate    = fmt.Errorf("run already exists for idempotency key")
)

type Record struct {
	RunID              string
	SessionKey         string
	InputEventID       string
	IdempotencyKey     string
	Status             string
	AutonomyMode       string
	CurrentState       string
	ModelProvider      string
	ProviderSessionRef string
	LeaseID            string
	ResumesRunID       string
	StartedAt          time.Time
	UpdatedAt          time.Time
	FinishedAt         *time.Time
	ErrorType          string
	ErrorMessage       string
	MetadataJSON       string
}

type Repository interface {
	CreateRun(ctx context.Context, record Record) (Record, error)
	GetRun(ctx context.Context, runID string) (Record, error)
	FindRunByIdempotencyKey(ctx context.Context, sessionKey, idempotencyKey string) (Record, error)
	UpdateRun(ctx context.Context, params UpdateParams) (Record, error)
}

type UpdateParams struct {
	RunID        string
	CurrentState string
	NextState    string
	Status       string
	LeaseID      string
	ErrorType    string
	ErrorMessage string
	FinishedAt   *time.Time
	UpdatedAt    time.Time
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateRun(ctx context.Context, record Record) (Record, error) {
	const query = `
		INSERT INTO runs (
			run_id, session_key, input_event_id, idempotency_key, status, autonomy_mode, current_state,
			model_provider, provider_session_ref, lease_id, resumes_run_id,
			started_at, updated_at, finished_at, error_type, error_message, metadata_json
		) VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, $6, $7,
			$8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17::jsonb
		)
		RETURNING run_id, session_key, input_event_id, COALESCE(idempotency_key, ''), status, autonomy_mode, current_state,
			model_provider, provider_session_ref, lease_id, resumes_run_id,
			started_at, updated_at, finished_at, error_type, error_message, metadata_json::text
	`

	stored, err := scanRunRow(r.pool.QueryRow(ctx, query,
		record.RunID,
		record.SessionKey,
		record.InputEventID,
		record.IdempotencyKey,
		record.Status,
		record.AutonomyMode,
		record.CurrentState,
		record.ModelProvider,
		nullString(record.ProviderSessionRef),
		nullString(record.LeaseID),
		nullString(record.ResumesRunID),
		record.StartedAt,
		record.UpdatedAt,
		record.FinishedAt,
		nullString(record.ErrorType),
		nullString(record.ErrorMessage),
		normalizeJSON(record.MetadataJSON),
	))
	if err != nil {
		if isForeignKeyViolation(err) {
			return Record{}, ErrSessionNotFound
		}
		if isUniqueViolation(err) {
			return Record{}, ErrRunDuplicate
		}
		return Record{}, fmt.Errorf("create run: %w", err)
	}
	return stored, nil
}

func (r *PostgresRepository) GetRun(ctx context.Context, runID string) (Record, error) {
	const query = `
		SELECT run_id, session_key, input_event_id, COALESCE(idempotency_key, ''), status, autonomy_mode, current_state,
			model_provider, provider_session_ref, lease_id, resumes_run_id,
			started_at, updated_at, finished_at, error_type, error_message, metadata_json::text
		FROM runs
		WHERE run_id = $1
	`

	record, err := scanRunRow(r.pool.QueryRow(ctx, query, runID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRunNotFound
		}
		return Record{}, fmt.Errorf("get run: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) FindRunByIdempotencyKey(ctx context.Context, sessionKey, idempotencyKey string) (Record, error) {
	const query = `
		SELECT run_id, session_key, input_event_id, COALESCE(idempotency_key, ''), status, autonomy_mode, current_state,
			model_provider, provider_session_ref, lease_id, resumes_run_id,
			started_at, updated_at, finished_at, error_type, error_message, metadata_json::text
		FROM runs
		WHERE session_key = $1 AND idempotency_key = $2
	`
	record, err := scanRunRow(r.pool.QueryRow(ctx, query, sessionKey, idempotencyKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRunNotFound
		}
		return Record{}, fmt.Errorf("find run by idempotency key: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) UpdateRun(ctx context.Context, params UpdateParams) (Record, error) {
	const query = `
		UPDATE runs
		SET status = $1,
			current_state = $2,
			lease_id = $3,
			error_type = $4,
			error_message = $5,
			finished_at = $6,
			updated_at = $7
		WHERE run_id = $8 AND current_state = $9
		RETURNING run_id, session_key, input_event_id, COALESCE(idempotency_key, ''), status, autonomy_mode, current_state,
			model_provider, provider_session_ref, lease_id, resumes_run_id,
			started_at, updated_at, finished_at, error_type, error_message, metadata_json::text
	`

	record, err := scanRunRow(r.pool.QueryRow(ctx, query,
		params.Status,
		params.NextState,
		nullString(params.LeaseID),
		nullString(params.ErrorType),
		nullString(params.ErrorMessage),
		params.FinishedAt,
		params.UpdatedAt,
		params.RunID,
		params.CurrentState,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRunNotFound
		}
		return Record{}, fmt.Errorf("update run: %w", err)
	}
	return record, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRunRow(row rowScanner) (Record, error) {
	var record Record
	var finishedAt *time.Time
	var providerSessionRef, leaseID, resumesRunID, errorType, errorMessage *string

	err := row.Scan(
		&record.RunID,
		&record.SessionKey,
		&record.InputEventID,
		&record.IdempotencyKey,
		&record.Status,
		&record.AutonomyMode,
		&record.CurrentState,
		&record.ModelProvider,
		&providerSessionRef,
		&leaseID,
		&resumesRunID,
		&record.StartedAt,
		&record.UpdatedAt,
		&finishedAt,
		&errorType,
		&errorMessage,
		&record.MetadataJSON,
	)
	if err != nil {
		return Record{}, err
	}
	if providerSessionRef != nil {
		record.ProviderSessionRef = *providerSessionRef
	}
	if leaseID != nil {
		record.LeaseID = *leaseID
	}
	if resumesRunID != nil {
		record.ResumesRunID = *resumesRunID
	}
	if finishedAt != nil {
		record.FinishedAt = finishedAt
	}
	if errorType != nil {
		record.ErrorType = *errorType
	}
	if errorMessage != nil {
		record.ErrorMessage = *errorMessage
	}
	return record, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func normalizeJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
