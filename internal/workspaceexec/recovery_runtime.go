package workspaceexec

import (
	"context"
	"sync"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
)

// Unexported execution proof can only be created by the inode-owning runtime
// after draining this exact copy and obtaining an independent physical receipt.
type recoveryExecutionKey struct{}
type recoveryExecution struct {
	runtime     *Runtime
	record      recoveryRecord
	observation workspace.Observation
}

func (c *RecoveryController) CheckReconciliationTx(ctx context.Context, tx pgx.Tx, d resources.Decision) error {
	proof, ok := ctx.Value(recoveryExecutionKey{}).(recoveryExecution)
	if !ok || proof.runtime == nil || proof.runtime.recovery != c || proof.record.phase != "erasing" || proof.observation.Kind != "erased" || proof.observation.WorkspaceID != proof.record.req.WorkspaceID || proof.observation.Digest == "" {
		return ErrDenied
	}
	r, t := proof.record, proof.record.target
	if d.OwnerID != r.owner || d.ResourceID != t.proof.ResourceID || d.LeaseID != t.proof.LeaseID || d.ExpectedVersion != t.proof.ExpectedResourceVersion || d.EvidenceID != "workspace-discard:"+r.id || proof.runtime.checkRootBinding() != nil {
		return ErrDenied
	}
	root, err := proof.runtime.owner.IdentityHash()
	if err != nil || root != t.root {
		return ErrDenied
	}
	var exact bool
	if tx.QueryRow(ctx, `SELECT q.phase='erasing' AND e.observation_hash=$2 AND e.root_identity_hash=$3 FROM code_workspace_recoveries q JOIN code_workspace_erasure_receipts e USING(recovery_id) WHERE q.recovery_id=$1 FOR SHARE OF q,e`, r.id, proof.observation.Digest, t.root).Scan(&exact) != nil || !exact {
		return ErrDenied
	}
	return c.current(ctx, tx, t, r.owner)
}

func (r *Runtime) loadRecovery(ctx context.Context, tx pgx.Tx, id string) (recoveryRecord, error) {
	var record recoveryRecord
	if tx.QueryRow(ctx, `SELECT recovery_id,owner_id,channel,delivery_id,phase,request_hash,workspace_id,resource_version,workspace_generation,task_revision,constraints_revision,previous_recovery_id FROM code_workspace_recoveries WHERE recovery_id=$1`, id).Scan(&record.id, &record.owner, &record.channel, &record.delivery, &record.phase, &record.requestHash, &record.req.WorkspaceID, &record.req.ResourceVersion, &record.req.WorkspaceGeneration, &record.req.TaskRevision, &record.req.ConstraintsRevision, &record.req.PreviousRecoveryID) != nil {
		return record, ErrRecoveryConflict
	}
	target, err := r.recovery.recordTarget(ctx, tx, record)
	record.target = target
	if err != nil || record.requestHash != requestDigest(record.req) {
		return record, ErrDenied
	}
	var profileMatches bool
	if tx.QueryRow(ctx, `SELECT a.profile_id=$2 FROM code_workspace_recoveries q JOIN code_workspace_dispatches d USING(ticket_id) JOIN code_workspace_authorizations a USING(authorization_id) WHERE q.recovery_id=$1`, id, r.dispatch.profile.ID).Scan(&profileMatches) != nil || !profileMatches {
		return record, ErrDenied
	}
	return record, nil
}
func recoveryActor(ctx context.Context, record recoveryRecord) context.Context {
	return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: record.owner, Channel: record.channel, DeliveryID: record.delivery})
}

// ProcessRecovery consumes only an already authenticated, immutable owner
// decision. No model tool or private RPC can manufacture one. Each attempt
// commits its one-use marker before physical erasure. A later invocation only
// observes absence; a surviving copy needs another explicit owner decision.
func (r *Runtime) ProcessRecovery(ctx context.Context, id string) error {
	return r.processRecovery(ctx, id, false)
}
func (r *Runtime) processRecovery(ctx context.Context, id string, try bool) error {
	if !opaqueID(id) {
		return ErrDenied
	}
	leave, ok := r.enter()
	if !ok {
		return ErrDenied
	}
	defer leave()
	tx, err := r.dispatch.pool.Begin(ctx)
	if err != nil {
		return ErrRecoveryConflict
	}
	record, err := r.loadRecovery(ctx, tx, id)
	_ = tx.Rollback(context.Background())
	if err != nil {
		return err
	}
	if record.phase == "discarded" {
		return nil
	}
	if record.phase != "requested" && record.phase != "erasing" {
		return ErrRecoveryConflict
	}
	release, err := r.copies.acquire(ctx, record.req.WorkspaceID, try)
	if err != nil {
		return err
	}
	defer release()
	if r.checkRootBinding() != nil {
		return ErrDenied
	}
	root, err := r.owner.IdentityHash()
	if err != nil || root != record.target.root {
		return ErrDenied
	}
	actorCtx := recoveryActor(ctx, record)
	// Commit-before-effect must not retain its PG locks/connection across the
	// next validation transaction. Failure to observe commit never erases.
	tx, err = r.dispatch.pool.Begin(actorCtx)
	if err != nil {
		return ErrRecoveryConflict
	}
	ownedAttempt := false
	if err = r.recovery.current(actorCtx, tx, record.target, record.owner); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	var phase string
	if tx.QueryRow(actorCtx, `SELECT phase FROM code_workspace_recoveries WHERE recovery_id=$1 FOR UPDATE`, id).Scan(&phase) != nil {
		_ = tx.Rollback(context.Background())
		return ErrRecoveryConflict
	}
	if phase == "requested" {
		if _, err = tx.Exec(actorCtx, `UPDATE code_workspace_recoveries SET phase='erasing',attempted_at=clock_timestamp() WHERE recovery_id=$1 AND phase='requested'`, id); err != nil {
			_ = tx.Rollback(context.Background())
			return ErrRecoveryConflict
		}
		ownedAttempt = true
	} else if phase != "erasing" {
		_ = tx.Rollback(context.Background())
		return ErrRecoveryConflict
	}
	if tx.Commit(actorCtx) != nil {
		return ErrRecoveryConflict
	}
	record.phase = "erasing"
	tx, err = r.dispatch.pool.Begin(actorCtx)
	if err != nil {
		return ErrRecoveryConflict
	}
	defer tx.Rollback(context.Background())
	if err = r.recovery.current(actorCtx, tx, record.target, record.owner); err != nil {
		return err
	}
	if tx.QueryRow(actorCtx, `SELECT phase FROM code_workspace_recoveries WHERE recovery_id=$1 FOR UPDATE`, id).Scan(&phase) != nil || phase != "erasing" || r.checkRootBinding() != nil {
		return ErrDenied
	}
	// Current owner/tree/source/resource locks stay held through bounded local
	// erasure, excluding amendment/revocation between validation and effect.
	// There is no RPC, second connection, provider call or global task lock.
	if ownedAttempt && r.purge.PurgeWorkspace(actorCtx, record.req.WorkspaceID) != nil {
		return ErrRecoveryConflict
	}
	observation, err := r.observer.Erased(actorCtx, record.req.WorkspaceID)
	if err != nil || r.checkRootBinding() != nil {
		return ErrRecoveryConflict
	}
	if _, err = tx.Exec(actorCtx, `INSERT INTO code_workspace_erasure_receipts(recovery_id,observation_hash,root_identity_hash) VALUES($1,$2,$3)`, id, observation.Digest, root); err != nil {
		return ErrRecoveryConflict
	}
	reconcileCtx := context.WithValue(actorCtx, recoveryExecutionKey{}, recoveryExecution{r, record, observation})
	resourceStore, err := resources.New(resources.WorkerGuard{}, r.recovery)
	if err != nil {
		return ErrDenied
	}
	if _, err = resourceStore.ReconcileTx(reconcileCtx, tx, record.owner, record.target.proof.ResourceID, record.target.proof.LeaseID, "workspace-discard:"+id, record.req.ResourceVersion); err != nil {
		return err
	}
	b := record.target.source.ref.Binding
	if record.req.WorkspaceGeneration == 0 {
		_, err = tx.Exec(actorCtx, `INSERT INTO code_workspaces(workspace_id,owner_id,session_key,task_id,task_revision,constraints_revision,state,generation,blocked_reason) VALUES($1,$2,$3,$4,$5,$6,'blocked',1,'owner_discarded')`, record.req.WorkspaceID, b.OwnerID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision)
	} else {
		var count int64
		tag, e := tx.Exec(actorCtx, `UPDATE code_workspaces SET state='blocked',generation=generation+1,blocked_reason='owner_discarded' WHERE workspace_id=$1 AND generation=$2`, record.req.WorkspaceID, record.req.WorkspaceGeneration)
		err = e
		count = tag.RowsAffected()
		if err == nil && count != 1 {
			err = ErrRecoveryConflict
		}
	}
	if err != nil {
		return ErrRecoveryConflict
	}
	if _, err = tx.Exec(actorCtx, `UPDATE code_workspace_recoveries SET phase='discarded',finished_at=clock_timestamp() WHERE recovery_id=$1 AND phase='erasing'`, id); err != nil {
		return ErrRecoveryConflict
	}
	if r.checkRootBinding() != nil || ctx.Err() != nil {
		return ErrRecoveryConflict
	}
	if tx.Commit(actorCtx) != nil {
		return ErrRecoveryConflict
	}
	// The original action/dispatch and every spent reservation remain unknown.
	return nil
}

func (r *Runtime) RunOwnerRecovery(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Transient fair scan cursor is scheduling only, never execution authority.
	// Unknown observation-only rows cannot permanently hide a later owner choice.
	after := ""
	for {
		bounded, stop := context.WithTimeout(ctx, 5*time.Second)
		rows, err := r.dispatch.pool.Query(bounded, `SELECT q.recovery_id FROM code_workspace_recoveries q JOIN code_workspace_dispatches d USING(ticket_id) JOIN code_workspace_authorizations a USING(authorization_id) WHERE q.phase IN ('requested','erasing') AND a.profile_id=$1 AND q.recovery_id>$2 ORDER BY q.recovery_id LIMIT 16`, r.dispatch.profile.ID, after)
		var ids []string
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
		}
		stop()
		if len(ids) == 0 {
			after = ""
		} else {
			after = ids[len(ids)-1]
		}
		jobs := make(chan string, len(ids))
		for _, id := range ids {
			jobs <- id
		}
		close(jobs)
		var workers sync.WaitGroup
		for worker := 0; worker < 2; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for id := range jobs {
					if ctx.Err() != nil {
						return
					}
					bounded, stop := context.WithTimeout(ctx, 5*time.Second)
					_ = r.processRecovery(bounded, id, true)
					stop()
				}
			}()
		}
		workers.Wait()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// The installed guard can never be satisfied by an Actor, lease renewal,
// fabricated receipt string, model verdict or API request alone.
var _ resources.OwnerGuard = (*RecoveryController)(nil)
