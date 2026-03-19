package chunks

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MemoryType   = "chunk"
	StatusActive = "active"

	// EffectiveStatus values (chunks don't have confirmation)
	EffectiveStatusActive     = "active"
	EffectiveStatusSuppressed = "suppressed"
	EffectiveStatusDeleted    = "deleted"
)

var (
	ErrStoreNotConfigured = fmt.Errorf("chunk memory store is not configured")
	ErrChunkNotFound      = fmt.Errorf("chunk memory entry not found")
)

type Store struct{ pool *pgxpool.Pool }

type Chunk struct {
	ID              int64
	MemoryType      string
	ScopeType       string
	ScopeID         string
	Title           string
	Content         string
	Summary         string
	SourceType      string
	SourceID        string
	ProvenanceJSON  string
	TagsJSON        string
	Confidence      float64
	Status          string
	Embedding       []float32
	EffectiveStatus string
	Suppressed      bool
	ExpiresAt       *time.Time
	EditedBy        string
	EditedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
	if len(chunk.Embedding) != 0 && len(chunk.Embedding) != embeddings.VectorDimensions() {
		return Chunk{}, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions())
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
	if strings.TrimSpace(chunk.EffectiveStatus) == "" {
		chunk.EffectiveStatus = EffectiveStatusActive
	}
	stored := Chunk{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_chunks (
			memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance, tags, confidence, status, embedding,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11, $12, NULLIF($13, '')::vector, $14, $15, $16, $17, $18, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, chunk.MemoryType, chunk.ScopeType, chunk.ScopeID, chunk.Title, chunk.Content, chunk.Summary, chunk.SourceType, chunk.SourceID, chunk.ProvenanceJSON, chunk.TagsJSON, chunk.Confidence, chunk.Status, vectorLiteral(chunk.Embedding),
		chunk.EffectiveStatus, chunk.Suppressed, chunk.ExpiresAt, chunk.EditedBy, chunk.EditedAt).Scan(
		&stored.ID, &stored.MemoryType, &stored.ScopeType, &stored.ScopeID, &stored.Title, &stored.Content, &stored.Summary, &stored.SourceType, &stored.SourceID, &stored.ProvenanceJSON, &stored.TagsJSON, &stored.Confidence, &stored.Status,
		&stored.EffectiveStatus, &stored.Suppressed, &stored.ExpiresAt, &stored.EditedBy, &stored.EditedAt, &stored.CreatedAt, &stored.UpdatedAt,
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
	if len(embedding) != embeddings.VectorDimensions() {
		return nil, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions())
	}
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at, embedding <=> $3::vector AS distance
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
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status,
			&item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt, &item.Distance); err != nil {
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
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status,
			&item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk title result: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *Store) GetByScope(ctx context.Context, scopeType, scopeID string, limit int) ([]Chunk, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_chunks
		WHERE scope_type = $1 AND scope_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), limit)
	if err != nil {
		return nil, fmt.Errorf("query chunk scope entries: %w", err)
	}
	defer rows.Close()
	var results []Chunk
	for rows.Next() {
		var item Chunk
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status,
			&item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk scope result: %w", err)
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

// GetByID returns a chunk by its ID.
func (s *Store) GetByID(ctx context.Context, id int64) (Chunk, error) {
	if s == nil || s.pool == nil {
		return Chunk{}, ErrStoreNotConfigured
	}
	return scanChunk(s.pool.QueryRow(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_chunks
		WHERE id = $1
	`, id))
}

// GetByScopeEffective returns chunks that are effective (visible in retrieval).
func (s *Store) GetByScopeEffective(ctx context.Context, scopeType, scopeID string, limit int) ([]Chunk, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_chunks
		WHERE scope_type = $1 AND scope_id = $2 AND effective_status = $3 AND suppressed = FALSE
			AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
		LIMIT $4
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), EffectiveStatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("query effective chunk entries: %w", err)
	}
	defer rows.Close()
	var results []Chunk
	for rows.Next() {
		item, err := scanChunk(rows)
		if err != nil {
			return nil, fmt.Errorf("scan effective chunk entry: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// Suppress soft-suppresses a chunk (hidden from retrieval but kept for audit).
func (s *Store) Suppress(ctx context.Context, id int64, editedBy string) (Chunk, error) {
	if s == nil || s.pool == nil {
		return Chunk{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanChunk(s.pool.QueryRow(ctx, `
		UPDATE memory_chunks
		SET suppressed = TRUE, effective_status = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND suppressed = FALSE
		RETURNING id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, EffectiveStatusSuppressed, editedBy, now))
}

// Unsuppress restores a suppressed chunk.
func (s *Store) Unsuppress(ctx context.Context, id int64, editedBy string) (Chunk, error) {
	if s == nil || s.pool == nil {
		return Chunk{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanChunk(s.pool.QueryRow(ctx, `
		UPDATE memory_chunks
		SET suppressed = FALSE, effective_status = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND suppressed = TRUE
		RETURNING id, memory_type, scope_type, scope_id, title, content, summary, source_type, source_id, provenance::text, tags::text, confidence, status,
			effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, EffectiveStatusActive, editedBy, now))
}

// HardDelete permanently removes a chunk from the database.
func (s *Store) HardDelete(ctx context.Context, id int64) error {
	if s == nil || s.pool == nil {
		return ErrStoreNotConfigured
	}
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM memory_chunks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("hard delete chunk: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrChunkNotFound
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanChunk(row rowScanner) (Chunk, error) {
	var item Chunk
	err := row.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Title, &item.Content, &item.Summary, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.TagsJSON, &item.Confidence, &item.Status,
		&item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Chunk{}, ErrChunkNotFound
		}
		return Chunk{}, err
	}
	return item, nil
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
