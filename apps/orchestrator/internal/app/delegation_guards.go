package app

import (
	"context"
	"reflect"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

// DelegationCapabilities is a private kernel snapshot, never model input. The
// reader must join the caller transaction and validate current registry/package
// metadata AND the parent's permission to delegate each exact capability. An
// old approval, a tool description or an operator registry entry alone is not
// that permission. Network calls and a second pooled connection are forbidden.
type DelegationCapabilities struct {
	TaskID, OwnerID                   string
	TaskRevision, ConstraintsRevision int64
	Permissions                       []delegation.Permission
	Budget                            delegation.Limits
	Deadline                          time.Time
}
type DelegationPermissionReader interface {
	// Empty childTaskID means a new proposal: require current delegate-dispatch
	// proof. A nonempty ID means lifecycle: resolve its durable accepted snapshot
	// and current policy, without requiring the old parent process/worker lease.
	ReadDelegationCapabilitiesTx(context.Context, pgx.Tx, task.Record, string) (DelegationCapabilities, error)
}

// Catalog owns grant ancestry, source locks and restore/deletion validation.
type DelegationCatalog interface {
	CheckDelegationProposalSourcesTx(context.Context, pgx.Tx, string, string, []catalog.SourceRef, string) error
	BindDelegationSourcesTx(context.Context, pgx.Tx, string, string, string, []catalog.SourceRef, string) error
	CheckDelegationSourcesTx(context.Context, pgx.Tx, string, []catalog.SourceRef, string) error
	AccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) ([]catalog.Source, error)
	CanonicalSourcesTx(context.Context, pgx.Tx, []catalog.SourceRef) ([]catalog.SourceRef, error)
	CheckRunSourcesTx(context.Context, pgx.Tx, string) error
}

type DelegationGuards struct {
	sources     DelegationCatalog
	permissions DelegationPermissionReader
}

func NewDelegationGuards(s DelegationCatalog, p DelegationPermissionReader) (*DelegationGuards, error) {
	if nilDelegationAdapter(s) || nilDelegationAdapter(p) {
		return nil, delegation.ErrDenied
	}
	return &DelegationGuards{s, p}, nil
}

func nilDelegationAdapter(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Func, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}

var _ task.DelegationGuard = (*DelegationGuards)(nil)

func refsForDelegation(c delegation.Contract) []catalog.SourceRef {
	return catalogRefs(append(append([]delegation.SourceRef(nil), c.Sources...), c.ContractSource))
}
func catalogRefs(refs []delegation.SourceRef) []catalog.SourceRef {
	out := make([]catalog.SourceRef, len(refs))
	for i, r := range refs {
		out[i] = catalog.SourceRef{ID: r.ID, Revision: r.Revision, AccessRevision: r.AccessRevision, Hash: r.Hash}
	}
	return out
}

func delegationNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var at time.Time
	err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at)
	return at, err
}

func currentDelegationTask(ctx context.Context, tx pgx.Tx, t task.Record) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND session_key=$3 AND status='active' AND revision=$4 AND constraints_revision=$5 AND execution_policy=$6 FROM tasks WHERE task_id=$1 FOR SHARE`, t.TaskID, t.OwnerID, t.SessionKey, t.Revision, t.ConstraintsRevision, t.ExecutionPolicy).Scan(&valid)
	if err != nil || !valid {
		return delegation.ErrDenied
	}
	return nil
}

func (g *DelegationGuards) CheckProposalTx(ctx context.Context, tx pgx.Tx, parent task.Record, r task.DelegationRequest) error {
	if r.ParentTaskID != parent.TaskID || r.ParentRevision != parent.Revision || r.ConstraintsRevision != parent.ConstraintsRevision {
		return delegation.ErrDenied
	}
	if err := currentDelegationTask(ctx, tx, parent); err != nil {
		return err
	}
	at, err := delegationNow(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, at); err != nil {
		return err
	}
	if err = g.sources.CheckDelegationProposalSourcesTx(ctx, tx, parent.TaskID, parent.OwnerID, catalogRefs(append(append([]delegation.SourceRef(nil), r.Sources...), r.ContractSource)), "memory_read"); err != nil {
		return err
	}
	return g.checkPermissionsTx(ctx, tx, parent, "", r.Permissions, r.Budget, r.Deadline)
}

func (g *DelegationGuards) BindContractSourcesTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	return g.sources.BindDelegationSourcesTx(ctx, tx, parent.OwnerID, parent.TaskID, child.TaskID, refsForDelegation(c), "memory_read")
}

func (g *DelegationGuards) CheckLifecycleTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	if c.ParentTaskID != parent.TaskID || c.TaskID != child.TaskID || child.ParentTaskID != parent.TaskID || child.OwnerID != parent.OwnerID || c.ParentRevision != parent.Revision || c.ConstraintsRevision != parent.ConstraintsRevision {
		return delegation.ErrDenied
	}
	if err := currentDelegationTask(ctx, tx, parent); err != nil {
		return err
	}
	if err := currentDelegationTask(ctx, tx, child); err != nil {
		return err
	}
	// The parent's current authority is sufficient for a waiting child. Requiring
	// a child worker or the old parent lease here would break durable recovery.
	if err := g.sources.CheckDelegationSourcesTx(ctx, tx, child.TaskID, refsForDelegation(c), "memory_read"); err != nil {
		return err
	}
	return g.checkPermissionsTx(ctx, tx, parent, c.TaskID, c.Permissions, c.Budget, c.Deadline)
}

func (g *DelegationGuards) checkPermissionsTx(ctx context.Context, tx pgx.Tx, parent task.Record, childTaskID string, permissions []delegation.Permission, allowance delegation.Limits, deadline time.Time) error {
	cap, err := g.permissions.ReadDelegationCapabilitiesTx(ctx, tx, parent, childTaskID)
	if err != nil {
		return err
	}
	at, err := delegationNow(ctx, tx)
	if err != nil {
		return err
	}
	if cap.TaskID != parent.TaskID || cap.OwnerID != parent.OwnerID || cap.TaskRevision != parent.Revision || cap.ConstraintsRevision != parent.ConstraintsRevision || !cap.Deadline.After(at) || !deadline.After(at) || deadline.After(cap.Deadline) || deadline.After(at.Add(24*time.Hour)) || !delegationBudgetSubset(allowance, cap.Budget) {
		return delegation.ErrDenied
	}
	// Reuse the strict immutable contract validator for exact, unique, bounded
	// resources/effects/aliases/hashes. No wildcard capability is representable.
	validation := delegation.Contract{TaskID: "validation-child", RootTaskID: parent.TaskID, ParentTaskID: parent.TaskID, ParentRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, Revision: 1, ContractSource: delegation.SourceRef{ID: "validation-source", Revision: 1, AccessRevision: 1, Hash: validationHash}, Budget: allowance, Deadline: deadline, Depth: 1, Permissions: permissions}
	if err = delegation.Validate(validation, at); err != nil {
		return err
	}
	validation.Permissions, validation.Budget, validation.Deadline = cap.Permissions, cap.Budget, at.Add(time.Hour)
	if err = delegation.Validate(validation, at); err != nil {
		return err
	}
	a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, parent.TaskID, parent.OwnerID, nil, at)
	if err != nil {
		return err
	}
	if a.TaskID != "" {
		m, goal, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
		if err != nil {
			return err
		}
		bound, err := actionauth.AuthorityDeadline(m, goal, at)
		if err != nil || deadline.After(bound) {
			return delegation.ErrDenied
		}
		if !delegationBudgetSubset(allowance, delegation.Limits{ModelTurns: m.Budget.ModelTurns, Tokens: m.Budget.Tokens, CostMicros: m.Budget.CostMicros, ExternalCalls: m.Budget.ExternalCalls, Children: m.Budget.Children, ActiveMillis: m.Budget.ActiveMillis, EgressBytes: m.Budget.EgressBytes, ProviderInputBytes: m.Budget.ProviderInputBytes}) {
			return delegation.ErrDenied
		}
	}
	for _, p := range permissions {
		found := false
		for _, allowed := range cap.Permissions {
			if permissionSubset(p, allowed) {
				found = true
				break
			}
		}
		if !found {
			return delegation.ErrDenied
		}
		credentials := make([]actionauth.CredentialRef, len(p.Credentials))
		for i, r := range p.Credentials {
			credentials[i] = actionauth.CredentialRef{Alias: r.Alias, Field: r.Field}
		}
		for _, resource := range p.Resources {
			for _, effect := range p.Effects {
				b := actionauth.ActionBinding{TaskID: parent.TaskID, TaskRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, OwnerID: parent.OwnerID, ToolName: p.Tool, ContractHash: p.ContractHash, Resource: resource, Effect: actionauth.Effect(effect), CredentialRefs: credentials, GoalID: a.GoalID, GoalRevision: a.GoalRevision, MandateID: a.MandateID, MandateRevision: a.MandateRevision, Purpose: a.Purpose, OwnerPolicyRevision: a.PolicyRevision}
				if err = actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: parent.ExecutionPolicy}, b.Effect); err != nil {
					return err
				}
				if err = actionauth.CheckTaskActionTx(ctx, tx, b, at); err != nil {
					return err
				}
			}
		}
	}
	return currentDelegationTask(ctx, tx, parent)
}

const validationHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func permissionSubset(p, allowed delegation.Permission) bool {
	if p.Tool != allowed.Tool || p.ContractHash != allowed.ContractHash {
		return false
	}
	for _, r := range p.Resources {
		if !hasExact(allowed.Resources, r) {
			return false
		}
	}
	for _, e := range p.Effects {
		if !hasExact(allowed.Effects, e) {
			return false
		}
	}
	for _, r := range p.Credentials {
		found := false
		for _, a := range allowed.Credentials {
			if a == r {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func hasExact(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func delegationBudgetSubset(a, b delegation.Limits) bool {
	x, y := delegationUnits(a), delegationUnits(b)
	for i, v := range x {
		if v < 0 || y[i] < 0 || v > y[i] {
			return false
		}
	}
	return true
}
func delegationUnits(u delegation.Limits) [8]int64 {
	return [8]int64{u.ModelTurns, u.Tokens, u.CostMicros, u.ExternalCalls, u.Children, u.ActiveMillis, u.EgressBytes, u.ProviderInputBytes}
}
