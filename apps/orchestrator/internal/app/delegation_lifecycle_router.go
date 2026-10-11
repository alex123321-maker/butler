package app

import (
	"context"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/jackc/pgx/v5"
)

// NewDelegationLifecycleManager composes current context and maintenance for
// both accepted roles. Admissions retain their separate fixed-contract managers;
// this manager cannot bind a new child or choose a role from caller input.
func NewDelegationLifecycleManager(store *task.Store, sessions task.DelegationSessions, inputs task.DelegationInputs, sources DelegationCatalog, limits task.DelegationLimits, initial *DelegationAdmission, developer *ConverterDelegation) (*task.DelegationManager, error) {
	if initial == nil || initial.reader == nil || initial.Manager == nil || developer == nil || developer.reader == nil || developer.Manager == nil {
		return nil, delegation.ErrDenied
	}
	reader := &delegationLifecycleReader{initial.reader, developer.reader}
	guard, err := NewDelegationGuards(sources, reader)
	if err != nil {
		return nil, err
	}
	return task.NewDelegationManager(store, sessions, inputs, &delegationLifecycleGuard{DelegationGuards: guard, reader: reader}, limits)
}

type delegationLifecycleReader struct {
	initial   *delegationAdmissionReader
	developer *converterDelegationReader
}

type delegationLifecycleGuard struct {
	*DelegationGuards
	reader *delegationLifecycleReader
}

func (g *delegationLifecycleGuard) BindContractSourcesTx(context.Context, pgx.Tx, task.Record, task.Record, delegation.Contract) error {
	return delegation.ErrDenied
}

func (r *delegationLifecycleReader) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	// Lifecycle selection depends only on immutable accepted records in this
	// transaction. In particular, an authority denial never tries another reader.
	if childID == "" {
		return DelegationCapabilities{}, delegation.ErrDenied
	}
	var initial, developer bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delegation_admission_snapshots WHERE task_id=$1),EXISTS(SELECT 1 FROM converter_developer_snapshots WHERE task_id=$1)`, childID).Scan(&initial, &developer)
	if err != nil || initial == developer {
		return DelegationCapabilities{}, delegation.ErrDenied
	}
	if developer {
		return r.developer.ReadDelegationCapabilitiesTx(ctx, tx, parent, childID)
	}
	return r.initial.ReadDelegationCapabilitiesTx(ctx, tx, parent, childID)
}
