package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusActive     = "active"
	StatusSuperseded = "superseded"
	MemoryType       = "profile"
)

var (
	ErrEntryNotFound      = fmt.Errorf("profile memory entry not found")
	ErrEntryConflict      = fmt.Errorf("active profile memory entry already exists")
	ErrScopeKeyChanged    = fmt.Errorf("superseded profile entry must keep the same scope and key")
	ErrStoreNotConfigured = fmt.Errorf("profile memory store is not configured")
)

type Store struct {
	pool *pgxpool.Pool
}

type Entry struct {
	ID             int64
	MemoryType     string
	ScopeType      string
	ScopeID        string
	Key            string
	ValueJSON      string
	Summary        string
	SourceType     string
	SourceID       string
	ProvenanceJSON string
	Confidence     float64
	Status         string
	EffectiveFrom  *time.Time
	EffectiveTo    *time.Time
	SupersedesID   *int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	if s == nil || s.pool == nil {
		return Entry{}, ErrStoreNotConfigured
	}
	entry, err := normalizeEntry(entry)
	if err != nil {
		return Entry{}, err
	}
	stored, err := scanEntry(s.pool.QueryRow(ctx, `
		INSERT INTO memory_profile (
			memory_type, scope_type, scope_id, key, value_json, summary, source_type, source_id, provenance, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
	`, entry.MemoryType, entry.ScopeType, entry.ScopeID, entry.Key, entry.ValueJSON, entry.Summary, entry.SourceType, entry.SourceID, entry.ProvenanceJSON, entry.Confidence, entry.Status, entry.EffectiveFrom, entry.EffectiveTo, entry.SupersedesID))
	if err != nil {
		if isUniqueViolation(err) {
			return Entry{}, ErrEntryConflict
		}
		return Entry{}, fmt.Errorf("save profile memory entry: %w", err)
	}
	return stored, nil
}

func (s *Store) Get(ctx context.Context, scopeType, scopeID, key string) (Entry, error) {
	if s == nil || s.pool == nil {
		return Entry{}, ErrStoreNotConfigured
	}
	return scanEntry(s.pool.QueryRow(ctx, `
		SELECT id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		FROM memory_profile
		WHERE scope_type = $1 AND scope_id = $2 AND key = $3 AND status = $4
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), strings.TrimSpace(key), StatusActive))
}

func (s *Store) Supersede(ctx context.Context, previousID int64, entry Entry) (Entry, error) {
	if s == nil || s.pool == nil {
		return Entry{}, ErrStoreNotConfigured
	}
	if previousID == 0 {
		return Entry{}, fmt.Errorf("previous entry id is required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Entry{}, fmt.Errorf("begin supersede profile entry: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	previous, err := scanEntry(tx.QueryRow(ctx, `
		SELECT id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		FROM memory_profile
		WHERE id = $1
	`, previousID))
	if err != nil {
		return Entry{}, fmt.Errorf("load superseded profile entry: %w", err)
	}
	if previous.Status != StatusActive {
		return Entry{}, fmt.Errorf("profile memory entry %d is not active", previousID)
	}
	entry = inheritScope(entry, previous)
	if entry.ScopeType != previous.ScopeType || entry.ScopeID != previous.ScopeID || entry.Key != previous.Key {
		return Entry{}, ErrScopeKeyChanged
	}
	entry.SupersedesID = &previous.ID
	entry, err = normalizeEntry(entry)
	if err != nil {
		return Entry{}, err
	}
	effectiveTo := time.Now().UTC()
	if entry.EffectiveFrom != nil {
		effectiveTo = entry.EffectiveFrom.UTC()
	}
	if _, err := tx.Exec(ctx, `
		UPDATE memory_profile
		SET status = $2,
			effective_to = $3,
			updated_at = NOW()
		WHERE id = $1
	`, previous.ID, StatusSuperseded, effectiveTo); err != nil {
		return Entry{}, fmt.Errorf("mark superseded profile entry: %w", err)
	}
	stored, err := scanEntry(tx.QueryRow(ctx, `
		INSERT INTO memory_profile (
			memory_type, scope_type, scope_id, key, value_json, summary, source_type, source_id, provenance, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, NOW(), NOW())
		RETURNING id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
	`, entry.MemoryType, entry.ScopeType, entry.ScopeID, entry.Key, entry.ValueJSON, entry.Summary, entry.SourceType, entry.SourceID, entry.ProvenanceJSON, entry.Confidence, entry.Status, entry.EffectiveFrom, entry.EffectiveTo, entry.SupersedesID))
	if err != nil {
		if isUniqueViolation(err) {
			return Entry{}, ErrEntryConflict
		}
		return Entry{}, fmt.Errorf("insert superseding profile entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Entry{}, fmt.Errorf("commit supersede profile entry: %w", err)
	}
	return stored, nil
}

func (s *Store) GetByScope(ctx context.Context, scopeType, scopeID string) ([]Entry, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		FROM memory_profile
		WHERE scope_type = $1 AND scope_id = $2 AND status = $3
		ORDER BY key ASC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), StatusActive)
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

func (s *Store) GetHistory(ctx context.Context, scopeType, scopeID, key string) ([]Entry, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, memory_type, scope_type, scope_id, key, value_json::text, summary, source_type, source_id, provenance::text, confidence, status, effective_from, effective_to, supersedes_id, created_at, updated_at
		FROM memory_profile
		WHERE scope_type = $1 AND scope_id = $2 AND key = $3
		ORDER BY created_at ASC
	`, strings.TrimSpace(scopeType), strings.TrimSpace(scopeID), strings.TrimSpace(key))
	if err != nil {
		return nil, fmt.Errorf("query profile memory history: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan profile memory history: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile memory history: %w", err)
	}
	return entries, nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanEntry(row rowScanner) (Entry, error) {
	var entry Entry
	err := row.Scan(&entry.ID, &entry.MemoryType, &entry.ScopeType, &entry.ScopeID, &entry.Key, &entry.ValueJSON, &entry.Summary, &entry.SourceType, &entry.SourceID, &entry.ProvenanceJSON, &entry.Confidence, &entry.Status, &entry.EffectiveFrom, &entry.EffectiveTo, &entry.SupersedesID, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Entry{}, ErrEntryNotFound
		}
		return Entry{}, err
	}
	return entry, nil
}

func normalizeEntry(entry Entry) (Entry, error) {
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
	if strings.TrimSpace(entry.ProvenanceJSON) == "" {
		entry.ProvenanceJSON = defaultProvenanceJSON(entry.SourceType, entry.SourceID)
	}
	if strings.TrimSpace(entry.MemoryType) == "" {
		entry.MemoryType = MemoryType
	}
	if entry.Confidence <= 0 {
		entry.Confidence = 1
	}
	if strings.TrimSpace(entry.Status) == "" {
		entry.Status = StatusActive
	}
	return entry, nil
}

func defaultProvenanceJSON(sourceType, sourceID string) string {
	return fmt.Sprintf(`{"source_type":%q,"source_id":%q}`, strings.TrimSpace(sourceType), strings.TrimSpace(sourceID))
}

func inheritScope(entry, previous Entry) Entry {
	if strings.TrimSpace(entry.ScopeType) == "" {
		entry.ScopeType = previous.ScopeType
	}
	if strings.TrimSpace(entry.ScopeID) == "" {
		entry.ScopeID = previous.ScopeID
	}
	if strings.TrimSpace(entry.Key) == "" {
		entry.Key = previous.Key
	}
	return entry
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
