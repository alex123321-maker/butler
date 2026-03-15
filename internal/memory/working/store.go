package working

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSnapshotNotFound = fmt.Errorf("working memory snapshot not found")

type Store struct {
	pool *pgxpool.Pool
}

type Snapshot struct {
	ID               int64
	SessionKey       string
	RunID            string
	Goal             string
	EntitiesJSON     string
	PendingStepsJSON string
	ScratchJSON      string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Save(ctx context.Context, snapshot Snapshot) (Snapshot, error) {
	if strings.TrimSpace(snapshot.SessionKey) == "" {
		return Snapshot{}, fmt.Errorf("session_key is required")
	}
	if snapshot.EntitiesJSON == "" {
		snapshot.EntitiesJSON = "{}"
	}
	if snapshot.PendingStepsJSON == "" {
		snapshot.PendingStepsJSON = "[]"
	}
	if snapshot.ScratchJSON == "" {
		snapshot.ScratchJSON = "{}"
	}
	if snapshot.Status == "" {
		snapshot.Status = "active"
	}

	const query = `
		INSERT INTO memory_working (session_key, run_id, goal, entities, pending_steps, scratch, status, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4::jsonb, $5::jsonb, $6::jsonb, $7, NOW(), NOW())
		ON CONFLICT (session_key)
		DO UPDATE SET
			run_id = EXCLUDED.run_id,
			goal = EXCLUDED.goal,
			entities = EXCLUDED.entities,
			pending_steps = EXCLUDED.pending_steps,
			scratch = EXCLUDED.scratch,
			status = EXCLUDED.status,
			updated_at = NOW()
		RETURNING id, session_key, COALESCE(run_id, ''), goal, entities::text, pending_steps::text, scratch::text, status, created_at, updated_at
	`
	stored, err := scanSnapshot(s.pool.QueryRow(ctx, query,
		snapshot.SessionKey,
		snapshot.RunID,
		snapshot.Goal,
		snapshot.EntitiesJSON,
		snapshot.PendingStepsJSON,
		snapshot.ScratchJSON,
		snapshot.Status,
	))
	if err != nil {
		return Snapshot{}, fmt.Errorf("save working memory snapshot: %w", err)
	}
	return stored, nil
}

func (s *Store) Get(ctx context.Context, sessionKey string) (Snapshot, error) {
	const query = `
		SELECT id, session_key, COALESCE(run_id, ''), goal, entities::text, pending_steps::text, scratch::text, status, created_at, updated_at
		FROM memory_working
		WHERE session_key = $1
	`
	snapshot, err := scanSnapshot(s.pool.QueryRow(ctx, query, strings.TrimSpace(sessionKey)))
	if err != nil {
		if err == pgx.ErrNoRows {
			return Snapshot{}, ErrSnapshotNotFound
		}
		return Snapshot{}, fmt.Errorf("get working memory snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *Store) Clear(ctx context.Context, sessionKey string) error {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM memory_working WHERE session_key = $1`, strings.TrimSpace(sessionKey))
	if err != nil {
		return fmt.Errorf("clear working memory snapshot: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(row rowScanner) (Snapshot, error) {
	var snapshot Snapshot
	err := row.Scan(
		&snapshot.ID,
		&snapshot.SessionKey,
		&snapshot.RunID,
		&snapshot.Goal,
		&snapshot.EntitiesJSON,
		&snapshot.PendingStepsJSON,
		&snapshot.ScratchJSON,
		&snapshot.Status,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
	)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}
