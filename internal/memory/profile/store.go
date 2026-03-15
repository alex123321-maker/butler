package profile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrEntryNotFound = fmt.Errorf("profile memory entry not found")

type Store struct {
	pool *pgxpool.Pool
}

type Entry struct {
	ID            int64
	ScopeType     string
	ScopeID       string
	Key           string
	ValueJSON     string
	Summary       string
	SourceType    string
	SourceID      string
	Status        string
	EffectiveFrom *time.Time
	EffectiveTo   *time.Time
	SupersedesID  *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (e Entry) ProfileKey() string { return e.Key }

func (e Entry) ProfileSummary() string {
	if strings.TrimSpace(e.Summary) != "" {
		return e.Summary
	}
	return e.Key
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Save(ctx context.Context, entry Entry) (Entry, error) {
	if strings.TrimSpace(entry.ScopeType) == "" {
		return Entry{}, fmt.Errorf("scope_type is required")
	}
	if strings.TrimSpace(entry.ScopeID) == "" {
		return Entry{}, fmt.Errorf("scope_id is required")
	}
	if strings.TrimSpace(entry.Key) == "" {
		return Entry{}, fmt.Errorf("key is required")
	}
	if strings.TrimSpace(entry.ValueJSON) == "" {
		entry.ValueJSON = "{}"
	}
	if strings.TrimSpace(entry.Status) == "" {
		entry.Status = "active"
	}
	stored, err := scanEntry(s.pool.QueryRow(ctx, `
		INSERT INTO memory_profile (
			scope_type, scope_id, key, value_json, summary, source_type, source_id, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		ON CONFLICT (scope_type, scope_id, key)
		DO UPDATE SET value_json = EXCLUDED.value_json, summary = EXCLUDED.summary, source_type = EXCLUDED.source_type, source_id = EXCLUDED.source_id, status = EXCLUDED.status, effective_from = EXCLUDED.effective_from, effective_to = EXCLUDED.effective_to, supersedes_id = EXCLUDED.supersedes_id, updated_at = NOW()
		RETURNING id, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, status, effective_from, effective_to, supersedes_id, created_at, updated_at
	`, entry.ScopeType, entry.ScopeID, entry.Key, entry.ValueJSON, entry.Summary, entry.SourceType, entry.SourceID, entry.Status, entry.EffectiveFrom, entry.EffectiveTo, entry.SupersedesID))
	if err != nil {
		return Entry{}, fmt.Errorf("save profile memory entry: %w", err)
	}
	return stored, nil
}

func (s *Store) GetByScope(ctx context.Context, scopeType, scopeID string) ([]Entry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		FROM memory_profile
		WHERE scope_type = $1 AND scope_id = $2
		ORDER BY key ASC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID))
	if err != nil {
		return nil, fmt.Errorf("query profile memory entries: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan profile memory entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile memory entries: %w", err)
	}
	return entries, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanEntry(row rowScanner) (Entry, error) {
	var entry Entry
	err := row.Scan(&entry.ID, &entry.ScopeType, &entry.ScopeID, &entry.Key, &entry.ValueJSON, &entry.Summary, &entry.SourceType, &entry.SourceID, &entry.Status, &entry.EffectiveFrom, &entry.EffectiveTo, &entry.SupersedesID, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Entry{}, ErrEntryNotFound
		}
		return Entry{}, err
	}
	return entry, nil
}
