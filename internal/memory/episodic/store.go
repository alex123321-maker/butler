package episodic

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

type Episode struct {
	ID             int64
	ScopeType      string
	ScopeID        string
	Summary        string
	Content        string
	SourceType     string
	SourceID       string
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
	if strings.TrimSpace(episode.ScopeType) == "" {
		return Episode{}, fmt.Errorf("scope_type is required")
	}
	if strings.TrimSpace(episode.ScopeID) == "" {
		return Episode{}, fmt.Errorf("scope_id is required")
	}
	if strings.TrimSpace(episode.Summary) == "" {
		return Episode{}, fmt.Errorf("summary is required")
	}
	if len(episode.Embedding) == 0 {
		return Episode{}, fmt.Errorf("embedding is required")
	}
	if strings.TrimSpace(episode.Status) == "" {
		episode.Status = "active"
	}
	if strings.TrimSpace(episode.TagsJSON) == "" {
		episode.TagsJSON = "[]"
	}
	stored := Episode{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_episodes (
			scope_type, scope_id, summary, content, source_type, source_id, status, tags, embedding, episode_start_at, episode_end_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::vector, $10, $11, NOW(), NOW())
		RETURNING id, scope_type, scope_id, summary, content, source_type, source_id, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at
	`, episode.ScopeType, episode.ScopeID, episode.Summary, episode.Content, episode.SourceType, episode.SourceID, episode.Status, episode.TagsJSON, vectorLiteral(episode.Embedding), episode.EpisodeStartAt, episode.EpisodeEndAt).Scan(
		&stored.ID, &stored.ScopeType, &stored.ScopeID, &stored.Summary, &stored.Content, &stored.SourceType, &stored.SourceID, &stored.Status, &stored.TagsJSON, &stored.EpisodeStartAt, &stored.EpisodeEndAt, &stored.CreatedAt, &stored.UpdatedAt,
	)
	if err != nil {
		return Episode{}, fmt.Errorf("save episodic memory: %w", err)
	}
	stored.Embedding = append([]float32(nil), episode.Embedding...)
	return stored, nil
}

func (s *Store) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, scope_type, scope_id, summary, content, source_type, source_id, status, tags::text, episode_start_at, episode_end_at, created_at, updated_at, embedding <=> $3::vector AS distance
		FROM memory_episodes
		WHERE scope_type = $1 AND scope_id = $2
		ORDER BY embedding <=> $3::vector ASC
		LIMIT $4
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), vectorLiteral(embedding), limit)
	if err != nil {
		return nil, fmt.Errorf("search episodic memory: %w", err)
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.ID, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt, &item.CreatedAt, &item.UpdatedAt, &item.Distance); err != nil {
			return nil, fmt.Errorf("scan episodic search result: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodic search result: %w", err)
	}
	return results, nil
}

func vectorLiteral(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
