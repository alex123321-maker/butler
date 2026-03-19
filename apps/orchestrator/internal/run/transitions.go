package run

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StateTransition records a single run state transition for observability.
type StateTransition struct {
	ID             int64
	RunID          string
	FromState      string
	ToState        string
	TriggeredBy    string
	MetadataJSON   string
	TransitionedAt time.Time
}

// TransitionRepository persists and queries run state transitions.
type TransitionRepository interface {
	InsertTransition(ctx context.Context, t StateTransition) error
	ListTransitions(ctx context.Context, runID string) ([]StateTransition, error)
}

// PostgresTransitionRepository implements TransitionRepository using PostgreSQL.
type PostgresTransitionRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresTransitionRepository creates a new PostgresTransitionRepository.
func NewPostgresTransitionRepository(pool *pgxpool.Pool) *PostgresTransitionRepository {
	return &PostgresTransitionRepository{pool: pool}
}

// InsertTransition inserts a state transition record.
func (r *PostgresTransitionRepository) InsertTransition(ctx context.Context, t StateTransition) error {
	const query = `
		INSERT INTO run_state_transitions (run_id, from_state, to_state, triggered_by, metadata_json, transitioned_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
	`
	metaJSON := t.MetadataJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	if _, err := r.pool.Exec(ctx, query, t.RunID, t.FromState, t.ToState, t.TriggeredBy, metaJSON, t.TransitionedAt); err != nil {
		return fmt.Errorf("insert state transition: %w", err)
	}
	return nil
}

// ListTransitions returns all state transitions for a run, ordered by time.
func (r *PostgresTransitionRepository) ListTransitions(ctx context.Context, runID string) ([]StateTransition, error) {
	const query = `
		SELECT id, run_id, from_state, to_state, triggered_by, metadata_json::text, transitioned_at
		FROM run_state_transitions
		WHERE run_id = $1
		ORDER BY transitioned_at ASC, id ASC
	`
	rows, err := r.pool.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("list state transitions: %w", err)
	}
	defer rows.Close()

	var transitions []StateTransition
	for rows.Next() {
		var t StateTransition
		if err := rows.Scan(&t.ID, &t.RunID, &t.FromState, &t.ToState, &t.TriggeredBy, &t.MetadataJSON, &t.TransitionedAt); err != nil {
			return nil, fmt.Errorf("scan state transition: %w", err)
		}
		transitions = append(transitions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate state transitions: %w", err)
	}
	return transitions, nil
}
