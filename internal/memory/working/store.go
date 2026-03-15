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
var ErrStoreNotConfigured = fmt.Errorf("working memory store is not configured")

type Store struct {
	pool *pgxpool.Pool
}

type Snapshot struct {
	ID               int64
	MemoryType       string
	SessionKey       string
	RunID            string
	Goal             string
	EntitiesJSON     string
	PendingStepsJSON string
	ScratchJSON      string
	Status           string
	SourceType       string
	SourceID         string
	ProvenanceJSON   string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Save(ctx context.Context, snapshot Snapshot) (Snapshot, error) {
	if s == nil || s.pool == nil {
		return Snapshot{}, ErrStoreNotConfigured
	}
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
	if strings.TrimSpace(snapshot.MemoryType) == "" {
		snapshot.MemoryType = "working"
	}
	if strings.TrimSpace(snapshot.ProvenanceJSON) == "" {
		snapshot.ProvenanceJSON = fmt.Sprintf(`{"source_type":%q,"source_id":%q}`, strings.TrimSpace(snapshot.SourceType), strings.TrimSpace(snapshot.SourceID))
	}

	const query = `
		INSERT INTO memory_working (memory_type, session_key, run_id, goal, entities, pending_steps, scratch, status, source_type, source_id, provenance, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5::jsonb, $6::jsonb, $7::jsonb, $8, $9, $10, $11::jsonb, NOW(), NOW())
		ON CONFLICT (session_key)
		DO UPDATE SET
			memory_type = EXCLUDED.memory_type,
			run_id = EXCLUDED.run_id,
			goal = EXCLUDED.goal,
			entities = EXCLUDED.entities,
			pending_steps = EXCLUDED.pending_steps,
			scratch = EXCLUDED.scratch,
			status = EXCLUDED.status,
			source_type = EXCLUDED.source_type,
			source_id = EXCLUDED.source_id,
			provenance = EXCLUDED.provenance,
			updated_at = NOW()
		RETURNING id, memory_type, session_key, COALESCE(run_id, ''), goal, entities::text, pending_steps::text, scratch::text, status, source_type, source_id, provenance::text, created_at, updated_at
	`
	stored, err := scanSnapshot(s.pool.QueryRow(ctx, query,
		snapshot.MemoryType,
		snapshot.SessionKey,
		snapshot.RunID,
		snapshot.Goal,
		snapshot.EntitiesJSON,
		snapshot.PendingStepsJSON,
		snapshot.ScratchJSON,
		snapshot.Status,
		snapshot.SourceType,
		snapshot.SourceID,
		snapshot.ProvenanceJSON,
	))
	if err != nil {
		return Snapshot{}, fmt.Errorf("save working memory snapshot: %w", err)
	}
	return stored, nil
}

func (s *Store) Get(ctx context.Context, sessionKey string) (Snapshot, error) {
	if s == nil || s.pool == nil {
		return Snapshot{}, ErrStoreNotConfigured
	}
	const query = `
		SELECT id, memory_type, session_key, COALESCE(run_id, ''), goal, entities::text, pending_steps::text, scratch::text, status, source_type, source_id, provenance::text, created_at, updated_at
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
	if s == nil || s.pool == nil {
		return ErrStoreNotConfigured
	}
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
		&snapshot.MemoryType,
		&snapshot.SessionKey,
		&snapshot.RunID,
		&snapshot.Goal,
		&snapshot.EntitiesJSON,
		&snapshot.PendingStepsJSON,
		&snapshot.ScratchJSON,
		&snapshot.Status,
		&snapshot.SourceType,
		&snapshot.SourceID,
		&snapshot.ProvenanceJSON,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
	)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}
