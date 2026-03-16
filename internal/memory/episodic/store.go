package episodic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

const (
	StatusActive   = "active"
	StatusInactive = "inactive"
	MemoryType     = "episodic"
)

var ErrStoreNotConfigured = fmt.Errorf("episodic memory store is not configured")

type Episode struct {
	ID             int64
	MemoryType     string
	ScopeType      string
	ScopeID        string
	Summary        string
	Content        string
	SourceType     string
	SourceID       string
	ProvenanceJSON string
	Confidence     float64
	Status         string
	TagsJSON       string
	Embedding      []float32
	EpisodeStartAt *time.Time
	EpisodeEndAt   *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SearchResult struct {
	Episode
	Distance float64
}

func (r SearchResult) EpisodeSummary() string { return r.Summary }

func (r SearchResult) EpisodeDistance() float64 { return r.Distance }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Save(ctx context.Context, episode Episode) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	if strings.TrimSpace(episode.ScopeType) == "" {
		return Episode{}, fmt.Errorf("scope_type is required")
	}
	if strings.TrimSpace(episode.ScopeID) == "" {
		return Episode{}, fmt.Errorf("scope_id is required")
	}
	if strings.TrimSpace(episode.Summary) == "" {
		return Episode{}, fmt.Errorf("summary is required")
	}
	if len(episode.Embedding) != embeddings.VectorDimensions {
		return Episode{}, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions)
	}
	if strings.TrimSpace(episode.MemoryType) == "" {
		episode.MemoryType = MemoryType
	}
	if episode.Confidence <= 0 {
		episode.Confidence = 1
	}
	if strings.TrimSpace(episode.Status) == "" {
		episode.Status = StatusActive
	}
	if strings.TrimSpace(episode.TagsJSON) == "" {
		episode.TagsJSON = "[]"
	}
	if strings.TrimSpace(episode.ProvenanceJSON) == "" {
		episode.ProvenanceJSON = fmt.Sprintf(`{"source_type":%q,"source_id":%q}`, strings.TrimSpace(episode.SourceType), strings.TrimSpace(episode.SourceID))
	}
	stored := Episode{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_episodes (
			memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance, confidence, status, tags, embedding, episode_start_at, episode_end_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11::jsonb, $12::vector, $13, $14, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at
	`, episode.MemoryType, episode.ScopeType, episode.ScopeID, episode.Summary, episode.Content, episode.SourceType, episode.SourceID, episode.ProvenanceJSON, episode.Confidence, episode.Status, episode.TagsJSON, vectorLiteral(episode.Embedding), episode.EpisodeStartAt, episode.EpisodeEndAt).Scan(
		&stored.ID, &stored.MemoryType, &stored.ScopeType, &stored.ScopeID, &stored.Summary, &stored.Content, &stored.SourceType, &stored.SourceID, &stored.ProvenanceJSON, &stored.Confidence, &stored.Status, &stored.TagsJSON, &stored.EpisodeStartAt, &stored.EpisodeEndAt, &stored.CreatedAt, &stored.UpdatedAt,
	)
	if err != nil {
		return Episode{}, fmt.Errorf("save episodic memory: %w", err)
	}
	stored.Embedding = append([]float32(nil), episode.Embedding...)
	return stored, nil
}

func (s *Store) GetByScope(ctx context.Context, scopeType, scopeID string) ([]Episode, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at
		FROM memory_episodes
		WHERE scope_type = $1 AND scope_id = $2 AND status = $3
		ORDER BY created_at DESC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), StatusActive)
	if err != nil {
		return nil, fmt.Errorf("query episodic memory entries: %w", err)
	}
	defer rows.Close()
	var items []Episode
	for rows.Next() {
		var item Episode
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan episodic memory entry: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodic memory entries: %w", err)
	}
	return items, nil
}

func (s *Store) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]SearchResult, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 5
	}
	if len(embedding) != embeddings.VectorDimensions {
		return nil, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at, embedding <=> $3::vector AS distance
		FROM memory_episodes
		WHERE scope_type = $1 AND scope_id = $2 AND status = $4 AND embedding IS NOT NULL
		ORDER BY embedding <=> $3::vector ASC
		LIMIT $5
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), vectorLiteral(embedding), StatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("search episodic memory: %w", err)
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt, &item.CreatedAt, &item.UpdatedAt, &item.Distance); err != nil {
			return nil, fmt.Errorf("scan episodic search result: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodic search result: %w", err)
	}
	return results, nil
}

func (s *Store) FindBySummary(ctx context.Context, scopeType, scopeID, summary string) ([]Episode, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at
		FROM memory_episodes
		WHERE scope_type = $1 AND scope_id = $2 AND lower(summary) = lower($3) AND status = $4
		ORDER BY created_at DESC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), strings.TrimSpace(summary), StatusActive)
	if err != nil {
		return nil, fmt.Errorf("query episodic summary matches: %w", err)
	}
	defer rows.Close()
	var items []Episode
	for rows.Next() {
		var item Episode
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan episodic summary match: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodic summary matches: %w", err)
	}
	return items, nil
}

func MergeTagsJSON(values ...string) string {
	set := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(trimmed), &tags); err != nil {
			continue
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			set[tag] = struct{}{}
		}
	}
	if len(set) == 0 {
		return "[]"
	}
	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	data, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func vectorLiteral(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
