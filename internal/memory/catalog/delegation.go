package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

type DelegatedRunSources struct {
	SessionKey   string
	Contract     delegation.Contract
	Sources      []Source
	Dependencies map[string][]SourceRef
}

// ReadDelegatedRun derives the selected context from the actual current run.
// Caller-supplied task IDs, inherited owner actors and auxiliary job proofs
// cannot widen this worker-only body-release path. Ordinary runs return nil.
func (s *Store) ReadDelegatedRun(ctx context.Context, runID string) (*DelegatedRunSources, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return nil, ErrDenied
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return nil, ErrDenied
	}
	if _, job := ctx.Value(checkerKey{}).(AccessChecker); job {
		return nil, ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, runID, token.LeaseID, token.FenceEpoch).Scan(&valid); err != nil {
		return nil, ErrDenied
	}
	var taskID, session string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(task_id,''),session_key FROM runs WHERE run_id=$1 AND current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') FOR SHARE`, runID).Scan(&taskID, &session); err != nil || session != token.SessionKey {
		return nil, ErrDenied
	}
	c, err := delegation.LoadTx(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, tx.Commit(ctx)
	}
	sources, err := s.AccessTx(ctx, tx, delegationRoots(c), "memory_read")
	if err != nil {
		return nil, err
	}
	dependencies := make(map[string][]SourceRef, len(sources))
	bytes := 0
	for _, src := range sources {
		bytes += len(src.Title) + len(src.Body)
		if bytes > 64<<10 {
			return nil, fmt.Errorf("selected delegated source context exceeds the bounded allowance")
		}
		parents, err := s.CanonicalSourcesTx(ctx, tx, []SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}})
		if err != nil {
			return nil, err
		}
		dependencies[src.ID] = parents
	}
	if err = s.CheckDelegationSourcesTx(ctx, tx, taskID, delegationRoots(c), "memory_read"); err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, runID, token.LeaseID, token.FenceEpoch).Scan(&valid); err != nil {
		return nil, ErrDenied
	}
	return &DelegatedRunSources{SessionKey: session, Contract: *c, Sources: sources, Dependencies: dependencies}, tx.Commit(ctx)
}

// CheckDelegationProposalSourcesTx checks the actual parent's current source
// rights before admission. Owner permission to read a body does not widen the
// parent's purpose-bound project grants. It performs no body publication.
func (s *Store) CheckDelegationProposalSourcesTx(ctx context.Context, tx pgx.Tx, parentID, own string, refs []SourceRef, purpose string) error {
	if parentID == "" || own == "" || purpose != "memory_read" || len(refs) < 1 || len(refs) > 65 {
		return ErrDenied
	}
	if token, ok := ownership.TokenFromContext(ctx); ok {
		if _, actor := owneridentity.FromContext(ctx); actor {
			return ErrDenied
		}
		var current bool
		if err := tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership o JOIN runs r ON r.run_id=o.run_id WHERE o.session_key=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND r.task_id=$5 AND r.session_key=o.session_key AND r.current_state NOT IN ('completed','failed','cancelled','timed_out','finalizing') FOR SHARE OF o,r`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch, parentID).Scan(&current); err != nil {
			return ErrDenied
		}
	} else if actor, ok := owneridentity.FromContext(ctx); !ok || actor.OwnerID != own {
		return ErrDenied
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM tasks WHERE task_id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, parentID, own).Scan(&current); err != nil {
		return ErrDenied
	}
	if err := s.CheckRestoreTx(ctx, tx); err != nil {
		return err
	}
	if _, err := s.AccessTx(ctx, tx, refs, purpose); err != nil {
		return err
	}
	canonical, err := s.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil {
		return err
	}
	for _, ref := range canonical {
		src, err := getSourceTx(ctx, tx, ref.ID, false)
		if err != nil || src.OwnerID != own || src.Status != "active" || src.Origin.Sensitivity == content.Secret {
			return ErrDenied
		}
		if err = checkInheritedPermissionTx(ctx, tx, parentID, own, src, purpose, 0); err != nil {
			return err
		}
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, parentID, own, nil, now); err != nil {
		return err
	}
	if token, ok := ownership.TokenFromContext(ctx); ok {
		if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&current); err != nil {
			return ErrDenied
		}
	}
	return nil
}

// BindDelegationSourcesTx belongs to Catalog, rather than Task Service writing
// memory grants. The caller has already serialized child admission. The actual
// parent proof must be able to read every selected root; ancestry is retained
// for privacy but does not become a selectable body in the child context.
func (s *Store) BindDelegationSourcesTx(ctx context.Context, tx pgx.Tx, own, parentID, childID string, refs []SourceRef, purpose string) error {
	c, err := delegation.LoadTx(ctx, tx, childID)
	if err != nil || c == nil || c.ParentTaskID != parentID || purpose != "memory_read" {
		return ErrDenied
	}
	if !sameDelegationRoots(c, refs) {
		return ErrDenied
	}
	if actor, ok := owneridentity.FromContext(ctx); ok {
		if actor.OwnerID != own {
			return ErrDenied
		}
	} else if token, ok := ownership.TokenFromContext(ctx); ok {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&valid); err != nil {
			return ErrDenied
		}
		var parentRun bool
		if err = tx.QueryRow(ctx, `SELECT task_id=$2 AND session_key=$3 FROM runs WHERE run_id=$1 FOR SHARE`, token.RunID, parentID, token.SessionKey).Scan(&parentRun); err != nil || !parentRun {
			return ErrDenied
		}
	} else {
		return ErrDenied
	}
	var parentOwner, childOwner string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM tasks WHERE task_id=$1 FOR SHARE`, parentID).Scan(&parentOwner); err != nil || parentOwner != own {
		return ErrDenied
	}
	if err = tx.QueryRow(ctx, `SELECT owner_id,revision FROM tasks WHERE task_id=$1 AND parent_task_id=$2 AND status='active' FOR SHARE`, childID, parentID).Scan(&childOwner, &revision); err != nil || childOwner != own {
		return ErrDenied
	}
	// Owner admission still cannot mint a grant the parent task itself lacks.
	// Worker admission uses the actual parent lease through AccessTx.
	if _, err = s.AccessTx(ctx, tx, refs, purpose); err != nil {
		return err
	}
	canonical, err := s.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil {
		return err
	}
	for _, ref := range canonical {
		src, err := getSourceTx(ctx, tx, ref.ID, false)
		if err != nil || src.OwnerID != own {
			return ErrDenied
		}
		if err = checkInheritedPermissionTx(ctx, tx, parentID, own, src, purpose, 0); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO memory_scope_grants(task_id,source_id,task_revision,access_revision,purpose,owner_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(task_id,source_id) DO NOTHING`, childID, src.ID, revision, src.AccessRevision, purpose, own); err != nil {
			return err
		}
		if err = checkExactGrantTx(ctx, tx, childID, own, src, purpose); err != nil {
			return err
		}
	}
	if err = s.CheckDelegationSourcesTx(ctx, tx, childID, refs, purpose); err != nil {
		return err
	}
	if token, ok := ownership.TokenFromContext(ctx); ok {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FOR SHARE`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&valid); err != nil {
			return ErrDenied
		}
	}
	return nil
}

// CheckDelegationSourcesTx is a typed lifecycle/dispatch guard. It needs no
// still-running parent worker: current parent task, mandate, source and grant
// state remain authoritative after a parent-process crash. It grants no body
// access and never invents an owner actor for a worker or maintenance caller.
func (s *Store) CheckDelegationSourcesTx(ctx context.Context, tx pgx.Tx, childID string, refs []SourceRef, purpose string) error {
	c, err := delegation.LoadTx(ctx, tx, childID)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	if purpose != "memory_read" || !sameDelegationRoots(c, refs) {
		return ErrDenied
	}
	if err = s.CheckRestoreTx(ctx, tx); err != nil {
		return err
	}
	var own, parentOwn, state string
	var revision, constraints int64
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM tasks WHERE task_id=$1 AND parent_task_id=$2 AND status='active' FOR SHARE`, childID, c.ParentTaskID).Scan(&own); err != nil {
		return ErrDenied
	}
	if err = tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, c.ParentTaskID).Scan(&parentOwn, &state, &revision, &constraints); err != nil || parentOwn != own || state != "active" || revision != c.ParentRevision || constraints != c.ConstraintsRevision {
		return ErrDenied
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil || !c.Deadline.After(now) || !delegationLifecycleState(c.State) {
		return ErrDenied
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, c.ParentTaskID, own, nil, now); err != nil {
		return err
	}
	canonical, err := s.CanonicalSourcesTx(ctx, tx, refs)
	if err != nil {
		return err
	}
	for _, ref := range canonical {
		src, err := getSourceTx(ctx, tx, ref.ID, false)
		if err != nil || src.OwnerID != own || src.Status != "active" || src.Origin.Sensitivity == content.Secret || ref.Revision != 0 && src.Revision != ref.Revision || ref.AccessRevision != 0 && src.AccessRevision != ref.AccessRevision || ref.Hash != "" && src.Hash != ref.Hash {
			return ErrStale
		}
		if err = checkExactGrantTx(ctx, tx, childID, own, src, purpose); err != nil {
			return err
		}
		if err = checkInheritedPermissionTx(ctx, tx, c.ParentTaskID, own, src, purpose, 0); err != nil {
			return err
		}
	}
	// A source-row wait may outlive the finite contract deadline.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil || !c.Deadline.After(now) {
		return ErrDenied
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, c.ParentTaskID, own, nil, now); err != nil {
		return err
	}
	return nil
}

func delegationRoots(c *delegation.Contract) []SourceRef {
	refs := make([]SourceRef, 0, len(c.Sources)+1)
	for _, r := range append(append([]delegation.SourceRef(nil), c.Sources...), c.ContractSource) {
		refs = append(refs, SourceRef{r.ID, r.Revision, r.AccessRevision, r.Hash})
	}
	return refs
}

func sameDelegationRoots(c *delegation.Contract, refs []SourceRef) bool {
	want := delegationRoots(c)
	if len(want) != len(refs) {
		return false
	}
	seen := map[SourceRef]bool{}
	for _, r := range refs {
		if seen[r] {
			return false
		}
		seen[r] = true
	}
	for _, r := range want {
		if !seen[r] {
			return false
		}
	}
	return true
}

func delegationLifecycleState(state string) bool {
	return state == "waiting" || state == "queued" || state == "executing" || state == "result_ready"
}

func delegatedBodyAllowed(c *delegation.Contract, src Source, session string) bool {
	if src.ScopeType == "session" && src.ScopeID == session || src.ScopeType == "task" && src.ScopeID == c.TaskID {
		return true
	}
	for _, r := range delegationRoots(c) {
		if r.ID == src.ID {
			return true
		}
	}
	return false
}

func checkExactGrantTx(ctx context.Context, tx pgx.Tx, taskID, own string, src Source, purpose string) error {
	var yes bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM memory_scope_grants g JOIN tasks t USING(task_id) WHERE g.task_id=$1 AND g.source_id=$2 AND g.task_revision=t.revision AND g.access_revision=$3 AND g.owner_id=$4 AND g.purpose=$5 FOR SHARE OF g`, taskID, src.ID, src.AccessRevision, own, purpose).Scan(&yes); err != nil {
		return ErrDenied
	}
	return nil
}

func checkInheritedPermissionTx(ctx context.Context, tx pgx.Tx, taskID, own string, src Source, purpose string, depth int) error {
	if depth > 3 {
		return ErrDenied
	}
	var session string
	if err := tx.QueryRow(ctx, `SELECT session_key FROM tasks WHERE task_id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, taskID, own).Scan(&session); err != nil {
		return ErrDenied
	}
	c, err := delegation.LoadTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if src.ScopeType == "session" && src.ScopeID == session || src.ScopeType == "task" && src.ScopeID == taskID || c == nil && src.ScopeType == "user" && src.ScopeID == own {
		return nil
	}
	if err = checkExactGrantTx(ctx, tx, taskID, own, src, purpose); err != nil {
		return err
	}
	if c != nil {
		return checkInheritedPermissionTx(ctx, tx, c.ParentTaskID, own, src, purpose, depth+1)
	}
	return nil
}
