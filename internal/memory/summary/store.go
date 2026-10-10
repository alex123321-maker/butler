package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot records the exact transcript prefix supplied to the summarizer.
// It is historical context, never a source of task authority or constraints.
type Snapshot struct {
	SessionKey       string
	Summary          string
	ThroughMessageID string
	ThroughSequence  int64
	SourceRevision   int64
	FormatVersion    int
	UpdatedAt        time.Time
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) GetSummary(ctx context.Context, sessionKey string) (string, error) {
	snapshot, err := s.GetSummarySnapshot(ctx, sessionKey)
	return snapshot.Summary, err
}

func (s *Store) GetSummarySnapshot(ctx context.Context, sessionKey string) (Snapshot, error) {
	var snapshot Snapshot
	err := s.pool.QueryRow(ctx, `SELECT session_key, summary, through_message_id, through_sequence,
        source_revision, format_version, updated_at FROM session_summaries WHERE session_key=$1`, sessionKey).
		Scan(&snapshot.SessionKey, &snapshot.Summary, &snapshot.ThroughMessageID, &snapshot.ThroughSequence,
			&snapshot.SourceRevision, &snapshot.FormatVersion, &snapshot.UpdatedAt)
	if err == pgx.ErrNoRows {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("read covered session summary: %w", err)
	}
	return snapshot, nil
}

// SaveCoveredSummary rejects mismatched source identities and stale coverage.
// The head lock serializes publication with source edits/deletions and append.
func (s *Store) SaveCoveredSummary(ctx context.Context, snapshot Snapshot) (bool, error) {
	if strings.TrimSpace(snapshot.SessionKey) == "" || strings.TrimSpace(snapshot.Summary) == "" ||
		snapshot.ThroughMessageID == "" || snapshot.ThroughSequence <= 0 || snapshot.SourceRevision <= 0 || snapshot.FormatVersion != 1 {
		return false, fmt.Errorf("summary requires content and a verified transcript boundary")
	}
	tag, err := s.pool.Exec(ctx, `WITH owner AS MATERIALIZED (
        SELECT session_key FROM sessions WHERE session_key=$1 FOR KEY SHARE
    ), boundary AS MATERIALIZED (
        SELECT m.session_key, m.message_id, m.message_sequence FROM messages m JOIN owner o USING(session_key)
        WHERE m.message_id=$3 AND m.message_sequence=$4 FOR KEY SHARE OF m
    ), head AS (
        SELECT h.source_revision FROM transcript_heads h JOIN boundary b USING(session_key) FOR UPDATE OF h
    ) INSERT INTO session_summaries (session_key, summary, through_message_id, through_sequence, source_revision)
    SELECT m.session_key, $2, m.message_id, m.message_sequence, h.source_revision
    FROM boundary m CROSS JOIN head h WHERE h.source_revision=$5
    ON CONFLICT (session_key) DO UPDATE SET summary=EXCLUDED.summary,
        through_message_id=EXCLUDED.through_message_id, through_sequence=EXCLUDED.through_sequence,
        source_revision=EXCLUDED.source_revision, updated_at=NOW()
    WHERE session_summaries.source_revision < EXCLUDED.source_revision OR
        (session_summaries.source_revision=EXCLUDED.source_revision AND
         session_summaries.through_sequence < EXCLUDED.through_sequence)`,
		snapshot.SessionKey, snapshot.Summary, snapshot.ThroughMessageID, snapshot.ThroughSequence, snapshot.SourceRevision)
	if err != nil {
		return false, fmt.Errorf("publish covered session summary: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// UpdateSummary is retained for the old pipeline interface. Unversioned writes
// must never silently acquire a coverage boundary.
func (s *Store) UpdateSummary(context.Context, string, string) error {
	return fmt.Errorf("summary publication requires a transcript coverage boundary")
}
