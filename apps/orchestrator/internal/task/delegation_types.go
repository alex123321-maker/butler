package task

import (
	"context"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/delegation"
	"github.com/jackc/pgx/v5"
)

// These adapters are owned by Session/Input/Catalog/policy modules. Every call
// joins the admission transaction; none may borrow a second pooled connection.
type DelegationSessions interface {
	CreateSessionTx(context.Context, pgx.Tx, session.CreateSessionParams) (session.SessionRecord, bool, error)
}
type DelegationInputs interface {
	AcceptBoundedTx(context.Context, pgx.Tx, delegation.InputRequest) (string, error)
}
type DelegationGuard interface {
	CheckProposalTx(context.Context, pgx.Tx, Record, DelegationRequest) error
	BindContractSourcesTx(context.Context, pgx.Tx, Record, Record, delegation.Contract) error
	CheckLifecycleTx(context.Context, pgx.Tx, Record, Record, delegation.Contract) error
	CheckReportTx(context.Context, pgx.Tx, Record, Record, Result, delegation.SourceRef) error
	CheckDecisionTx(context.Context, pgx.Tx, Record, Record, Result, DelegationDecision) error
}

type DelegationLimits struct{ MaxDepth, MaxChildren, MaxParallel, MaxDependencies int }

func DefaultDelegationLimits() DelegationLimits { return DelegationLimits{3, 8, 1, 8} }

// A proposal references versioned data. Kernel IDs, authority and ancestry are
// derived from durable records and the current private execution context.
type DelegationRequest struct {
	ParentTaskID        string                  `json:"parent_task_id"`
	ParentRevision      int64                   `json:"parent_revision"`
	ConstraintsRevision int64                   `json:"parent_constraints_revision"`
	ContractSource      delegation.SourceRef    `json:"contract_source"`
	Sources             []delegation.SourceRef  `json:"source_refs"`
	Permissions         []delegation.Permission `json:"permissions"`
	Budget              delegation.Limits       `json:"budget"`
	Deadline            time.Time               `json:"deadline"`
	Dependencies        []string                `json:"dependencies"`
}

// Results and decisions contain only current catalog/immutable result evidence
// references. Report prose remains deletable Catalog data, never new authority.
type DelegationDecision struct {
	Accept   bool                   `json:"accept"`
	Report   delegation.SourceRef   `json:"report_source"`
	Evidence []delegation.SourceRef `json:"evidence_refs"`
}
type DelegationRecord struct {
	Contract      delegation.Contract   `json:"contract"`
	ResultID      string                `json:"result_id,omitempty"`
	Report        *delegation.SourceRef `json:"report_source,omitempty"`
	Decision      *DelegationDecision   `json:"decision,omitempty"`
	BlockedReason string                `json:"blocked_reason,omitempty"`
}
