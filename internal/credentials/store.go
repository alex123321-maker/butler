package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusActive  = "active"
	StatusRevoked = "revoked"

	ApprovalPolicyAlwaysConfirm     = "always_confirm"
	ApprovalPolicyConfirmOnMutation = "confirm_on_mutation"
	ApprovalPolicyAutoReadOnly      = "auto_read_only"
	ApprovalPolicyManualOnly        = "manual_only"
)

var (
	ErrAliasRequired       = errors.New("credential alias is required")
	ErrRecordNotFound      = errors.New("credential record not found")
	ErrRecordAlreadyExists = errors.New("credential alias already exists")
)

type Store struct {
	pool *pgxpool.Pool
}

type Record struct {
	ID             int64
	Alias          string
	SecretType     string
	TargetType     string
	AllowedDomains []string
	AllowedTools   []string
	ApprovalPolicy string
	SecretRef      string
	Status         string
	MetadataJSON   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, record Record) (Record, error) {
	record.Alias = normalizeAlias(record.Alias)
	record.ApprovalPolicy = strings.TrimSpace(record.ApprovalPolicy)
	record.Status = strings.TrimSpace(record.Status)
	record.SecretType = strings.TrimSpace(record.SecretType)
	record.TargetType = strings.TrimSpace(record.TargetType)
	record.SecretRef = strings.TrimSpace(record.SecretRef)
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	stored, err := scanRecord(s.pool.QueryRow(ctx, `
		INSERT INTO credentials (
			alias,
			secret_type,
			target_type,
			allowed_domains,
			allowed_tools,
			approval_policy,
			secret_ref,
			status,
			metadata,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6, $7, $8, $9::jsonb, NOW(), NOW())
		RETURNING id, alias, secret_type, target_type, allowed_domains::text, allowed_tools::text, approval_policy, secret_ref, status, metadata::text, created_at, updated_at
	`,
		record.Alias,
		record.SecretType,
		record.TargetType,
		mustJSONArray(record.AllowedDomains),
		mustJSONArray(record.AllowedTools),
		record.ApprovalPolicy,
		record.SecretRef,
		record.Status,
		normalizeMetadata(record.MetadataJSON),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return Record{}, ErrRecordAlreadyExists
		}
		return Record{}, fmt.Errorf("create credential record: %w", err)
	}
	return stored, nil
}

func (s *Store) GetByAlias(ctx context.Context, alias string) (Record, error) {
	alias = normalizeAlias(alias)
	if alias == "" {
		return Record{}, ErrAliasRequired
	}
	stored, err := scanRecord(s.pool.QueryRow(ctx, `
		SELECT id, alias, secret_type, target_type, allowed_domains::text, allowed_tools::text, approval_policy, secret_ref, status, metadata::text, created_at, updated_at
		FROM credentials
		WHERE alias = $1
	`, alias))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRecordNotFound
		}
		return Record{}, fmt.Errorf("get credential record: %w", err)
	}
	return stored, nil
}

func (s *Store) List(ctx context.Context, includeRevoked bool) ([]Record, error) {
	query := `
		SELECT id, alias, secret_type, target_type, allowed_domains::text, allowed_tools::text, approval_policy, secret_ref, status, metadata::text, created_at, updated_at
		FROM credentials
	`
	args := []any{}
	if !includeRevoked {
		query += ` WHERE status <> $1`
		args = append(args, StatusRevoked)
	}
	query += ` ORDER BY alias`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list credential records: %w", err)
	}
	defer rows.Close()

	var result []Record
	for rows.Next() {
		stored, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan credential record: %w", err)
		}
		result = append(result, stored)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credential records: %w", err)
	}
	return result, nil
}

func (s *Store) Update(ctx context.Context, record Record) (Record, error) {
	record.Alias = normalizeAlias(record.Alias)
	record.ApprovalPolicy = strings.TrimSpace(record.ApprovalPolicy)
	record.Status = strings.TrimSpace(record.Status)
	record.SecretType = strings.TrimSpace(record.SecretType)
	record.TargetType = strings.TrimSpace(record.TargetType)
	record.SecretRef = strings.TrimSpace(record.SecretRef)
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	stored, err := scanRecord(s.pool.QueryRow(ctx, `
		UPDATE credentials
		SET secret_type = $2,
			target_type = $3,
			allowed_domains = $4::jsonb,
			allowed_tools = $5::jsonb,
			approval_policy = $6,
			secret_ref = $7,
			status = $8,
			metadata = $9::jsonb,
			updated_at = NOW()
		WHERE alias = $1
		RETURNING id, alias, secret_type, target_type, allowed_domains::text, allowed_tools::text, approval_policy, secret_ref, status, metadata::text, created_at, updated_at
	`,
		record.Alias,
		record.SecretType,
		record.TargetType,
		mustJSONArray(record.AllowedDomains),
		mustJSONArray(record.AllowedTools),
		record.ApprovalPolicy,
		record.SecretRef,
		record.Status,
		normalizeMetadata(record.MetadataJSON),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRecordNotFound
		}
		return Record{}, fmt.Errorf("update credential record: %w", err)
	}
	return stored, nil
}

func (s *Store) Revoke(ctx context.Context, alias string) (Record, error) {
	alias = normalizeAlias(alias)
	if alias == "" {
		return Record{}, ErrAliasRequired
	}
	stored, err := scanRecord(s.pool.QueryRow(ctx, `
		UPDATE credentials
		SET status = $2,
			updated_at = NOW()
		WHERE alias = $1
		RETURNING id, alias, secret_type, target_type, allowed_domains::text, allowed_tools::text, approval_policy, secret_ref, status, metadata::text, created_at, updated_at
	`, alias, StatusRevoked))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrRecordNotFound
		}
		return Record{}, fmt.Errorf("revoke credential record: %w", err)
	}
	return stored, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	var allowedDomains string
	var allowedTools string
	err := row.Scan(
		&record.ID,
		&record.Alias,
		&record.SecretType,
		&record.TargetType,
		&allowedDomains,
		&allowedTools,
		&record.ApprovalPolicy,
		&record.SecretRef,
		&record.Status,
		&record.MetadataJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	if err := json.Unmarshal([]byte(allowedDomains), &record.AllowedDomains); err != nil {
		return Record{}, fmt.Errorf("decode allowed_domains: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedTools), &record.AllowedTools); err != nil {
		return Record{}, fmt.Errorf("decode allowed_tools: %w", err)
	}
	return record, nil
}

func validateRecord(record Record) error {
	record.Alias = normalizeAlias(record.Alias)
	if record.Alias == "" {
		return ErrAliasRequired
	}
	if strings.TrimSpace(record.SecretType) == "" {
		return errors.New("secret_type is required")
	}
	if strings.TrimSpace(record.TargetType) == "" {
		return errors.New("target_type is required")
	}
	policy := strings.TrimSpace(record.ApprovalPolicy)
	if policy == "" {
		return errors.New("approval_policy is required")
	}
	switch policy {
	case ApprovalPolicyAlwaysConfirm, ApprovalPolicyConfirmOnMutation, ApprovalPolicyAutoReadOnly, ApprovalPolicyManualOnly:
	default:
		return fmt.Errorf("approval_policy %q is not supported", policy)
	}
	status := strings.TrimSpace(record.Status)
	if status == "" {
		return errors.New("status is required")
	}
	switch status {
	case StatusActive, StatusRevoked:
	default:
		return fmt.Errorf("status %q is not supported", status)
	}
	if strings.TrimSpace(record.SecretRef) == "" {
		return errors.New("secret_ref is required")
	}
	if !json.Valid([]byte(normalizeMetadata(record.MetadataJSON))) {
		return errors.New("metadata_json must be valid JSON")
	}
	return nil
}

func mustJSONArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func normalizeMetadata(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func normalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
