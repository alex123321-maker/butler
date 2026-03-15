package provenance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

var ErrStoreNotConfigured = fmt.Errorf("memory provenance store is not configured")

type Link struct {
	ID               int64
	SourceMemoryType string
	SourceMemoryID   int64
	LinkType         string
	TargetType       string
	TargetID         string
	MetadataJSON     string
	CreatedAt        time.Time
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) SaveLink(ctx context.Context, link Link) (Link, error) {
	if s == nil || s.pool == nil {
		return Link{}, ErrStoreNotConfigured
	}
	link = normalizeLink(link)
	if err := validateLink(link); err != nil {
		return Link{}, err
	}
	stored := Link{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO memory_links (
			source_memory_type, source_memory_id, link_type, target_type, target_id, metadata
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		ON CONFLICT (source_memory_type, source_memory_id, link_type, target_type, target_id)
		DO UPDATE SET metadata = EXCLUDED.metadata
		RETURNING id, source_memory_type, source_memory_id, link_type, target_type, target_id, metadata::text, created_at
	`, link.SourceMemoryType, link.SourceMemoryID, link.LinkType, link.TargetType, link.TargetID, link.MetadataJSON).Scan(
		&stored.ID,
		&stored.SourceMemoryType,
		&stored.SourceMemoryID,
		&stored.LinkType,
		&stored.TargetType,
		&stored.TargetID,
		&stored.MetadataJSON,
		&stored.CreatedAt,
	)
	if err != nil {
		return Link{}, fmt.Errorf("save memory link: %w", err)
	}
	return stored, nil
}

func (s *Store) ListBySource(ctx context.Context, sourceMemoryType string, sourceMemoryID int64) ([]Link, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, source_memory_type, source_memory_id, link_type, target_type, target_id, metadata::text, created_at
		FROM memory_links
		WHERE source_memory_type = $1 AND source_memory_id = $2
		ORDER BY id ASC
	`, strings.TrimSpace(sourceMemoryType), sourceMemoryID)
	if err != nil {
		return nil, fmt.Errorf("query memory links: %w", err)
	}
	defer rows.Close()
	var links []Link
	for rows.Next() {
		var link Link
		if err := rows.Scan(&link.ID, &link.SourceMemoryType, &link.SourceMemoryID, &link.LinkType, &link.TargetType, &link.TargetID, &link.MetadataJSON, &link.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan memory link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory links: %w", err)
	}
	return links, nil
}

func normalizeLink(link Link) Link {
	link.SourceMemoryType = strings.TrimSpace(link.SourceMemoryType)
	link.LinkType = strings.TrimSpace(link.LinkType)
	link.TargetType = strings.TrimSpace(link.TargetType)
	link.TargetID = strings.TrimSpace(link.TargetID)
	if strings.TrimSpace(link.MetadataJSON) == "" {
		link.MetadataJSON = "{}"
	}
	return link
}

func validateLink(link Link) error {
	if link.SourceMemoryType == "" {
		return fmt.Errorf("source_memory_type is required")
	}
	if link.SourceMemoryID == 0 {
		return fmt.Errorf("source_memory_id is required")
	}
	if link.LinkType == "" {
		return fmt.Errorf("link_type is required")
	}
	if link.TargetType == "" {
		return fmt.Errorf("target_type is required")
	}
	if link.TargetID == "" {
		return fmt.Errorf("target_id is required")
	}
	return nil
}
