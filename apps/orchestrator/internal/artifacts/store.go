package artifacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TypeAssistantFinal = "assistant_final"
	TypeDoctorReport   = "doctor_report"
	TypeToolResult     = "tool_result"
	TypeSummary        = "summary"
)

type Record struct {
	ArtifactID    string
	RunID         string
	SessionKey    string
	ArtifactType  string
	Title         string
	Summary       string
	ContentText   string
	ContentJSON   string
	ContentFormat string
	SourceType    string
	SourceRef     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateParams struct {
	ArtifactID    string
	RunID         string
	SessionKey    string
	ArtifactType  string
	Title         string
	Summary       string
	ContentText   string
	ContentJSON   string
	ContentFormat string
	SourceType    string
	SourceRef     string
	CreatedAt     time.Time
}

type ListParams struct {
	ArtifactType string
	RunID        string
	SessionKey   string
	Query        string
	Limit        int
	Offset       int
}

type Repository interface {
	CreateArtifact(ctx context.Context, params CreateParams) (Record, error)
	GetArtifactByID(ctx context.Context, artifactID string) (Record, error)
	ListArtifacts(ctx context.Context, params ListParams) ([]Record, error)
	ListArtifactsByRun(ctx context.Context, runID string, limit int) ([]Record, error)
}

var ErrArtifactNotFound = fmt.Errorf("artifact not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateArtifact(ctx context.Context, params CreateParams) (Record, error) {
	if params.CreatedAt.IsZero() {
		params.CreatedAt = time.Now().UTC()
	}
	if params.ContentJSON == "" {
		params.ContentJSON = "{}"
	}
	if params.ContentFormat == "" {
		params.ContentFormat = "text"
	}

	const query = `
		INSERT INTO artifacts (
			artifact_id, run_id, session_key, artifact_type, title, summary,
			content_text, content_json, content_format, source_type, source_ref,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$12)
		ON CONFLICT (artifact_id) DO UPDATE SET
			title = EXCLUDED.title,
			summary = EXCLUDED.summary,
			content_text = EXCLUDED.content_text,
			content_json = EXCLUDED.content_json,
			content_format = EXCLUDED.content_format,
			source_type = EXCLUDED.source_type,
			source_ref = EXCLUDED.source_ref,
			updated_at = EXCLUDED.updated_at
		RETURNING artifact_id, run_id, session_key, artifact_type, title, summary, content_text, content_json::text, content_format, source_type, source_ref, created_at, updated_at
	`

	rec := Record{}
	err := r.pool.QueryRow(ctx, query,
		params.ArtifactID,
		params.RunID,
		params.SessionKey,
		params.ArtifactType,
		params.Title,
		params.Summary,
		params.ContentText,
		params.ContentJSON,
		params.ContentFormat,
		params.SourceType,
		params.SourceRef,
		params.CreatedAt,
	).Scan(
		&rec.ArtifactID,
		&rec.RunID,
		&rec.SessionKey,
		&rec.ArtifactType,
		&rec.Title,
		&rec.Summary,
		&rec.ContentText,
		&rec.ContentJSON,
		&rec.ContentFormat,
		&rec.SourceType,
		&rec.SourceRef,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return Record{}, fmt.Errorf("create artifact: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) GetArtifactByID(ctx context.Context, artifactID string) (Record, error) {
	const query = `
		SELECT artifact_id, run_id, session_key, artifact_type, title, summary, content_text, content_json::text, content_format, source_type, source_ref, created_at, updated_at
		FROM artifacts
		WHERE artifact_id = $1
	`
	rec := Record{}
	err := r.pool.QueryRow(ctx, query, artifactID).Scan(
		&rec.ArtifactID,
		&rec.RunID,
		&rec.SessionKey,
		&rec.ArtifactType,
		&rec.Title,
		&rec.Summary,
		&rec.ContentText,
		&rec.ContentJSON,
		&rec.ContentFormat,
		&rec.SourceType,
		&rec.SourceRef,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Record{}, ErrArtifactNotFound
		}
		return Record{}, fmt.Errorf("get artifact by id: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) ListArtifacts(ctx context.Context, params ListParams) ([]Record, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	queryTerm := "%" + params.Query + "%"
	const query = `
		SELECT artifact_id, run_id, session_key, artifact_type, title, summary, content_text, content_json::text, content_format, source_type, source_ref, created_at, updated_at
		FROM artifacts
		WHERE ($1 = '' OR artifact_type = $1)
		  AND ($2 = '' OR run_id = $2)
		  AND ($3 = '' OR session_key = $3)
		  AND ($4 = '%%' OR title ILIKE $4 OR summary ILIKE $4 OR content_text ILIKE $4)
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6
	`

	rows, err := r.pool.Query(ctx, query, params.ArtifactType, params.RunID, params.SessionKey, queryTerm, params.Limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	items := make([]Record, 0)
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ArtifactID, &rec.RunID, &rec.SessionKey, &rec.ArtifactType, &rec.Title, &rec.Summary, &rec.ContentText, &rec.ContentJSON, &rec.ContentFormat, &rec.SourceType, &rec.SourceRef, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan artifact row: %w", err)
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) ListArtifactsByRun(ctx context.Context, runID string, limit int) ([]Record, error) {
	return r.ListArtifacts(ctx, ListParams{RunID: runID, Limit: limit, Offset: 0})
}
