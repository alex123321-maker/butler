package chunks

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MemoryType   = "chunk"
	StatusActive = "active"
)

var ErrStoreNotConfigured = fmt.Errorf("chunk memory store is not configured")

type Store struct{ pool *pgxpool.Pool }

type Chunk struct {
	ID             int64
	MemoryType     string
	ScopeType      string
	ScopeID        string
	Title          string
	Content        string
	Summary        string
	SourceType     string
	SourceID       string
	ProvenanceJSON string
	TagsJSON       string
	Confidence     float64
	Status         string
	Embedding      []float32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SearchResult struct {
	Chunk
	Distance float64
}

func (r SearchResult) ChunkTitle() string     { return r.Title }
func (r SearchResult) ChunkSummary() string   { return r.Summary }
func (r SearchResult) ChunkDistance() float64 { return r.Distance }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Save(ctx context.Context, chunk Chunk) (Chunk, error) {
	if s == nil || s.pool == nil {
		return Chunk{}, ErrStoreNotConfigured
	}
	if strings.TrimSpace(chunk.ScopeType) == "" || strings.TrimSpace(chunk.ScopeID) == "" {
		return Chunk{}, fmt.Errorf("scope is required")
	}
	if strings.TrimSpace(chunk.Title) == "" || strings.TrimSpace(chunk.Content) == "" {
		return Chunk{}, fmt.Errorf("title and content are required")
	}
	if len(chunk.Embedding) != 0 && len(chunk.Embedding) != embeddings.VectorDimensions {
		return Chunk{}, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions)
	}
	if strings.TrimSpace(chunk.MemoryType) == "" {
		chunk.MemoryType = MemoryType
	}
	if strings.TrimSpace(chunk.Status) == "" {
		chunk.Status = StatusActive
	}
	if strings.TrimSpace(chunk.Summary) == "" {
		chunk.Summary = strings.TrimSpace(chunk.Title)
	}
	if strings.TrimSpace(chunk.TagsJSON) == "" {
		chunk.TagsJSON = "[]"
	}
	if strings.TrimSpace(chunk.ProvenanceJSON) == "" {
		chunk.ProvenanceJSON = fmt.Sprintf(`{"source_type":%q,"source_id":%q}`, strings.TrimSpace(chunk.SourceType), strings.TrimSpace(chunk.SourceID))
	}
	if chunk.Confidence <= 0 {
		chunk.Confidence = 1
	}
	stored := Chunk{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_chunks (
			memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance, tags, confidence, status, embedding, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11, $12, NULLIF($13, '')::vector, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status, created_at, updated_at
	`, chunk.MemoryType, chunk.ScopeType, chunk.ScopeID, chunk.Title, chunk.Content, chunk.Summary, chunk.SourceType, chunk.SourceID, chunk.ProvenanceJSON, chunk.TagsJSON, chunk.Confidence, chunk.Status, vectorLiteral(chunk.Embedding)).Scan(
		&stored.ID, &stored.MemoryType, &stored.ScopeType, &stored.ScopeID, &stored.Title, &stored.Content, &stored.Summary, &stored.SourceType, &stored.SourceID, &stored.ProvenanceJSON, &stored.TagsJSON, &stored.Confidence, &stored.Status, &stored.CreatedAt, &stored.UpdatedAt,
	)
	if err != nil {
		return Chunk{}, fmt.Errorf("save chunk memory: %w", err)
	}
	stored.Embedding = append([]float32(nil), chunk.Embedding...)
	return stored, nil
}

func (s *Store) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]SearchResult, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if len(embedding) != embeddings.VectorDimensions {
		return nil, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions)
	}
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status, created_at, updated_at, embedding <=> $3::vector AS distance
		FROM memory_chunks
		WHERE scope_type = $1 AND scope_id = $2 AND status = $4 AND embedding IS NOT NULL
		ORDER BY embedding <=> $3::vector ASC
		LIMIT $5
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), vectorLiteral(embedding), StatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("search chunk memory: %w", err)
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.Distance); err != nil {
			return nil, fmt.Errorf("scan chunk search result: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *Store) FindByTitle(ctx context.Context, scopeType, scopeID, title string, limit int) ([]Chunk, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status, created_at, updated_at
		FROM memory_chunks
		WHERE scope_type = $1 AND scope_id = $2 AND lower(title) = lower($3) AND status = $4
		ORDER BY created_at DESC
		LIMIT $5
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), strings.TrimSpace(title), StatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("query chunk titles: %w", err)
	}
	defer rows.Close()
	var results []Chunk
	for rows.Next() {
		var item Chunk
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk title result: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *Store) Prune(ctx context.Context, cutoff time.Time, keepPerScope int) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, ErrStoreNotConfigured
	}
	if keepPerScope <= 0 {
		keepPerScope = 20
	}
	commandTag, err := s.pool.Exec(ctx, `
		WITH ranked AS (
			SELECT id,
			       ROW_NUMBER() OVER (PARTITION BY scope_type, scope_id ORDER BY confidence DESC, created_at DESC) AS rn
			FROM memory_chunks
			WHERE created_at < $1
		)
		DELETE FROM memory_chunks
		WHERE id IN (
			SELECT id FROM ranked WHERE rn > $2
		)
		   OR (status <> $3 AND created_at < $1)
	`, cutoff.UTC(), keepPerScope, StatusActive)
	if err != nil {
		return 0, fmt.Errorf("prune chunk memory entries: %w", err)
	}
	return commandTag.RowsAffected(), nil
}

func vectorLiteral(values []float32) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
