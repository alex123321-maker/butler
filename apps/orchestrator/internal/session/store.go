package session

import (
	"context"
	"fmt"
	"time"

	"github.com/butler/butler/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Re-export domain sentinel so in-package references work unchanged.
var ErrSessionNotFound = domain.ErrSessionNotFound

type SessionRecord struct {
	SessionID    string
	SessionKey   string
	UserID       string
	Channel      string
	MetadataJSON string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Repository interface {
	CreateSession(ctx context.Context, params CreateSessionParams) (SessionRecord, bool, error)
	GetSessionByKey(ctx context.Context, sessionKey string) (SessionRecord, error)
	ListSessions(ctx context.Context, limit, offset int) ([]SessionRecord, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateSession(ctx context.Context, params CreateSessionParams) (SessionRecord, bool, error) {
	const insertQuery = `
		INSERT INTO sessions (session_key, user_id, channel, metadata)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (session_key) DO NOTHING
		RETURNING session_id, session_key, user_id, channel, metadata::text, created_at, updated_at
	`

	record, err := scanSessionRow(r.pool.QueryRow(ctx, insertQuery,
		params.SessionKey,
		params.UserID,
		params.Channel,
		params.MetadataJSON,
	))
	if err == nil {
		return record, true, nil
	}
	if err != pgx.ErrNoRows {
		return SessionRecord{}, false, fmt.Errorf("insert session: %w", err)
	}

	record, err = r.GetSessionByKey(ctx, params.SessionKey)
	if err != nil {
		return SessionRecord{}, false, err
	}

	return record, false, nil
}

func (r *PostgresRepository) GetSessionByKey(ctx context.Context, sessionKey string) (SessionRecord, error) {
	const query = `
		SELECT session_id, session_key, user_id, channel, metadata::text, created_at, updated_at
		FROM sessions
		WHERE session_key = $1
	`

	record, err := scanSessionRow(r.pool.QueryRow(ctx, query, sessionKey))
	if err != nil {
		if err == pgx.ErrNoRows {
			return SessionRecord{}, ErrSessionNotFound
		}
		return SessionRecord{}, fmt.Errorf("get session by key: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) ListSessions(ctx context.Context, limit, offset int) ([]SessionRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	const query = `
		SELECT session_id, session_key, user_id, channel, metadata::text, created_at, updated_at
		FROM sessions
		ORDER BY updated_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionRecord
	for rows.Next() {
		record, err := scanSessionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		sessions = append(sessions, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}

type sessionRowScanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(row sessionRowScanner) (SessionRecord, error) {
	var record SessionRecord
	err := row.Scan(
		&record.SessionID,
		&record.SessionKey,
		&record.UserID,
		&record.Channel,
		&record.MetadataJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return SessionRecord{}, err
	}
	return record, nil
}
