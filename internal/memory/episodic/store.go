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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

const (
	StatusActive   = "active"
	StatusInactive = "inactive"
	MemoryType     = "episodic"

	// ConfirmationState values
	ConfirmationPending       = "pending"
	ConfirmationConfirmed     = "confirmed"
	ConfirmationRejected      = "rejected"
	ConfirmationAutoConfirmed = "auto_confirmed"

	// EffectiveStatus values
	EffectiveStatusActive     = "active"
	EffectiveStatusInactive   = "inactive"
	EffectiveStatusSuppressed = "suppressed"
	EffectiveStatusExpired    = "expired"
	EffectiveStatusDeleted    = "deleted"
)

var (
	ErrStoreNotConfigured = fmt.Errorf("episodic memory store is not configured")
	ErrEpisodeNotFound    = fmt.Errorf("episodic memory entry not found")
)

type Episode struct {
	ID                int64
	MemoryType        string
	ScopeType         string
	ScopeID           string
	Summary           string
	Content           string
	SourceType        string
	SourceID          string
	ProvenanceJSON    string
	Confidence        float64
	Status            string
	TagsJSON          string
	Embedding         []float32
	EpisodeStartAt    *time.Time
	EpisodeEndAt      *time.Time
	ConfirmationState string
	EffectiveStatus   string
	Suppressed        bool
	ExpiresAt         *time.Time
	EditedBy          string
	EditedAt          *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	if len(episode.Embedding) != embeddings.VectorDimensions() {
		return Episode{}, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions())
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
	if strings.TrimSpace(episode.ConfirmationState) == "" {
		episode.ConfirmationState = ConfirmationAutoConfirmed
	}
	if strings.TrimSpace(episode.EffectiveStatus) == "" {
		episode.EffectiveStatus = EffectiveStatusActive
	}
	stored := Episode{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_episodes (
			memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance, confidence, status, tags, embedding, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11::jsonb, $12::vector, $13, $14, $15, $16, $17, $18, $19, $20, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, episode.MemoryType, episode.ScopeType, episode.ScopeID, episode.Summary, episode.Content, episode.SourceType, episode.SourceID, episode.ProvenanceJSON, episode.Confidence, episode.Status, episode.TagsJSON, vectorLiteral(episode.Embedding), episode.EpisodeStartAt, episode.EpisodeEndAt,
		episode.ConfirmationState, episode.EffectiveStatus, episode.Suppressed, episode.ExpiresAt, episode.EditedBy, episode.EditedAt).Scan(
		&stored.ID, &stored.MemoryType, &stored.ScopeType, &stored.ScopeID, &stored.Summary, &stored.Content, &stored.SourceType, &stored.SourceID, &stored.ProvenanceJSON, &stored.Confidence, &stored.Status, &stored.TagsJSON, &stored.EpisodeStartAt, &stored.EpisodeEndAt,
		&stored.ConfirmationState, &stored.EffectiveStatus, &stored.Suppressed, &stored.ExpiresAt, &stored.EditedBy, &stored.EditedAt, &stored.CreatedAt, &stored.UpdatedAt,
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
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt,
			&item.ConfirmationState, &item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
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
	if len(embedding) != embeddings.VectorDimensions() {
		return nil, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions())
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at, embedding <=> $3::vector AS distance
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
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt,
			&item.ConfirmationState, &item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt, &item.Distance); err != nil {
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
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt,
			&item.ConfirmationState, &item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan episodic summary match: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodic summary matches: %w", err)
	}
	return items, nil
}

// GetByID returns an episode by its ID.
func (s *Store) GetByID(ctx context.Context, id int64) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	return scanEpisode(s.pool.QueryRow(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_episodes
		WHERE id = $1
	`, id))
}

// GetByScopeEffective returns episodes that are effective (visible in retrieval).
func (s *Store) GetByScopeEffective(ctx context.Context, scopeType, scopeID string) ([]Episode, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_episodes
		WHERE scope_type = $1 AND scope_id = $2 AND effective_status = $3 AND suppressed = FALSE
			AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), EffectiveStatusActive)
	if err != nil {
		return nil, fmt.Errorf("query effective episodic memory entries: %w", err)
	}
	defer rows.Close()
	var items []Episode
	for rows.Next() {
		item, err := scanEpisode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan effective episodic memory entry: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate effective episodic memory entries: %w", err)
	}
	return items, nil
}

// ListPendingConfirmation returns episodes awaiting user confirmation.
func (s *Store) ListPendingConfirmation(ctx context.Context, limit int) ([]Episode, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
		FROM memory_episodes
		WHERE confirmation_state = $1 AND effective_status = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, ConfirmationPending, EffectiveStatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending confirmation episodes: %w", err)
	}
	defer rows.Close()
	var items []Episode
	for rows.Next() {
		item, err := scanEpisode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending confirmation episode: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending confirmation episodes: %w", err)
	}
	return items, nil
}

// Confirm marks an episode as confirmed by user.
func (s *Store) Confirm(ctx context.Context, id int64, editedBy string) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanEpisode(s.pool.QueryRow(ctx, `
		UPDATE memory_episodes
		SET confirmation_state = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND confirmation_state = $5
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, ConfirmationConfirmed, editedBy, now, ConfirmationPending))
}

// Reject marks an episode as rejected by user.
func (s *Store) Reject(ctx context.Context, id int64, editedBy string) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanEpisode(s.pool.QueryRow(ctx, `
		UPDATE memory_episodes
		SET confirmation_state = $2, effective_status = $3, edited_by = $4, edited_at = $5, updated_at = NOW()
		WHERE id = $1 AND confirmation_state = $6
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, ConfirmationRejected, EffectiveStatusInactive, editedBy, now, ConfirmationPending))
}

// Suppress soft-suppresses an episode (hidden from retrieval but kept for audit).
func (s *Store) Suppress(ctx context.Context, id int64, editedBy string) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanEpisode(s.pool.QueryRow(ctx, `
		UPDATE memory_episodes
		SET suppressed = TRUE, effective_status = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND suppressed = FALSE
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, EffectiveStatusSuppressed, editedBy, now))
}

// Unsuppress restores a suppressed episode.
func (s *Store) Unsuppress(ctx context.Context, id int64, editedBy string) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanEpisode(s.pool.QueryRow(ctx, `
		UPDATE memory_episodes
		SET suppressed = FALSE, effective_status = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND suppressed = TRUE
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, EffectiveStatusActive, editedBy, now))
}

// SoftDelete marks an episode as deleted (kept for audit).
func (s *Store) SoftDelete(ctx context.Context, id int64, editedBy string) (Episode, error) {
	if s == nil || s.pool == nil {
		return Episode{}, ErrStoreNotConfigured
	}
	now := time.Now().UTC()
	return scanEpisode(s.pool.QueryRow(ctx, `
		UPDATE memory_episodes
		SET effective_status = $2, edited_by = $3, edited_at = $4, updated_at = NOW()
		WHERE id = $1 AND effective_status <> $2
		RETURNING id, memory_type, scope_type, scope_id, summary, content, source_type, source_id, provenance::text, confidence, status, tags::text, episode_start_at, episode_end_at,
			confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at, created_at, updated_at
	`, id, EffectiveStatusDeleted, editedBy, now))
}

type rowScanner interface{ Scan(dest ...any) error }

func scanEpisode(row rowScanner) (Episode, error) {
	var item Episode
	err := row.Scan(&item.ID, &item.MemoryType, &item.ScopeType, &item.ScopeID, &item.Summary, &item.Content, &item.SourceType, &item.SourceID, &item.ProvenanceJSON, &item.Confidence, &item.Status, &item.TagsJSON, &item.EpisodeStartAt, &item.EpisodeEndAt,
		&item.ConfirmationState, &item.EffectiveStatus, &item.Suppressed, &item.ExpiresAt, &item.EditedBy, &item.EditedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Episode{}, ErrEpisodeNotFound
		}
		return Episode{}, err
	}
	return item, nil
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
			FROM memory_episodes
			WHERE created_at < $1
		)
		DELETE FROM memory_episodes
		WHERE id IN (
			SELECT id FROM ranked WHERE rn > $2
		)
		   OR (status <> $3 AND created_at < $1)
	`, cutoff.UTC(), keepPerScope, StatusActive)
	if err != nil {
		return 0, fmt.Errorf("prune episodic memory entries: %w", err)
	}
	return commandTag.RowsAffected(), nil
}

func vectorLiteral(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
