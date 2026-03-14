package session

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSessionNotFound = fmt.Errorf("session not found")

type SessionRecord struct {
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
		RETURNING session_key, user_id, channel, metadata::text, created_at, updated_at
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
		SELECT session_key, user_id, channel, metadata::text, created_at, updated_at
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

type sessionRowScanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(row sessionRowScanner) (SessionRecord, error) {
	var record SessionRecord
	err := row.Scan(
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
