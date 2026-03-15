package credentials

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AuditDecisionAllowed  = "allowed"
	AuditDecisionDenied   = "denied"
	AuditDecisionNotFound = "not_found"
)

type AuditLogger interface {
	Create(context.Context, AuditLog) error
}

type AuditStore struct {
	pool *pgxpool.Pool
}

type AuditLog struct {
	RunID        string
	ToolCallID   string
	Alias        string
	Field        string
	ToolName     string
	TargetDomain string
	Decision     string
	CreatedAt    time.Time
}

func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

func (s *AuditStore) Create(ctx context.Context, entry AuditLog) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("credential audit store is not configured")
	}
	entry.Alias = strings.TrimSpace(entry.Alias)
	entry.Field = strings.TrimSpace(entry.Field)
	entry.ToolName = strings.TrimSpace(entry.ToolName)
	entry.TargetDomain = strings.TrimSpace(strings.ToLower(entry.TargetDomain))
	entry.Decision = strings.TrimSpace(entry.Decision)
	if entry.Alias == "" {
		return fmt.Errorf("alias is required")
	}
	if entry.Field == "" {
		return fmt.Errorf("field is required")
	}
	if entry.ToolName == "" {
		return fmt.Errorf("tool_name is required")
	}
	if entry.Decision != AuditDecisionAllowed && entry.Decision != AuditDecisionDenied && entry.Decision != AuditDecisionNotFound {
		return fmt.Errorf("decision %q is not supported", entry.Decision)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO credential_audit_logs (
			run_id,
			tool_call_id,
			alias,
			field,
			tool_name,
			target_domain,
			decision,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, strings.TrimSpace(entry.RunID), strings.TrimSpace(entry.ToolCallID), entry.Alias, entry.Field, entry.ToolName, entry.TargetDomain, entry.Decision); err != nil {
		return fmt.Errorf("create credential audit log: %w", err)
	}
	return nil
}
