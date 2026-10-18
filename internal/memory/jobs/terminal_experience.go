package jobs

import (
	"context"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// This persisted proof is deliberately absent from ModelPolicyRequest. The
// ordinary HTTP/model policy decoder cannot request permission for a blocked
// task. Only the owning experience adapter below can construct this capability;
// every later admission, supervision and publication rechecks its sealed proof.
type terminalExperiencePin struct {
	SourceID        string `json:"source_id"`
	ObservationHash string `json:"observation_hash"`
	Reconciled      bool   `json:"reconciled,omitempty"`
}

func (s *Store) CreateTerminalExperiencePolicy(ctx context.Context, q ModelPolicyRequest, snapshot string) (ModelPolicy, error) {
	return s.createStoppedExperiencePolicy(ctx, q, snapshot, false)
}

func (s *Store) CreateReconciledExperiencePolicy(ctx context.Context, q ModelPolicyRequest, snapshot string) (ModelPolicy, error) {
	return s.createStoppedExperiencePolicy(ctx, q, snapshot, true)
}

func (s *Store) createStoppedExperiencePolicy(ctx context.Context, q ModelPolicyRequest, snapshot string, reconciled bool) (ModelPolicy, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID == "" || a.DeliveryID == "" || snapshot == "" || len(q.Job.Sources) != 1 || q.Job.Sources[0].ID != snapshot || q.Handler != "task-experience-v1" || q.Trigger != "source_changed" || q.MaxJobs != 1 || q.Job.Allowance.ModelTurns != 1 || q.Job.TaskID == "" || q.Job.OriginalRunID == "" || len(q.Contexts) != 0 {
		return ModelPolicy{}, ErrClaim
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ModelPolicy{}, err
	}
	defer tx.Rollback(ctx)
	pin := &terminalExperiencePin{SourceID: snapshot, Reconciled: reconciled}
	if reconciled {
		err = tx.QueryRow(ctx, `SELECT o.observation_hash FROM memory_experience_observations o JOIN memory_experience_reconciled_attempts a USING(source_id)
WHERE o.source_id=$1 AND o.owner_id=$2 AND o.task_id=$3 AND o.run_id=$4 AND o.result_id='' AND butler_memory_reconciled_attempt_current(o.source_id)`, snapshot, a.OwnerID, q.Job.TaskID, q.Job.OriginalRunID).Scan(&pin.ObservationHash)
	} else {
		err = tx.QueryRow(ctx, `SELECT o.observation_hash FROM memory_experience_observations o JOIN memory_experience_terminal_attempts a USING(source_id)
WHERE o.source_id=$1 AND o.owner_id=$2 AND o.task_id=$3 AND o.run_id=$4 AND o.result_id='' AND butler_memory_terminal_attempt_current(o.source_id)`, snapshot, a.OwnerID, q.Job.TaskID, q.Job.OriginalRunID).Scan(&pin.ObservationHash)
	}
	if err != nil {
		return ModelPolicy{}, ErrClaim
	}
	p, err := s.createModelPolicyTx(ctx, tx, q, pin)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func (s *Store) checkExperienceTerminalTaskTx(ctx context.Context, tx pgx.Tx, p ModelPolicy, status string) error {
	if p.TerminalExperience == nil {
		if status != "active" && status != "completed" {
			return ErrClaim
		}
		if p.Handler == "task-experience-v1" && len(p.Job.Sources) > 0 {
			ids := make([]string, len(p.Job.Sources))
			for i, ref := range p.Job.Sources {
				ids[i] = ref.ID
			}
			var terminal bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_terminal_attempts WHERE source_id=ANY($1::text[])) OR EXISTS(SELECT 1 FROM memory_experience_reconciled_attempts WHERE source_id=ANY($1::text[]))`, ids).Scan(&terminal); err != nil {
				return err
			}
			if terminal {
				return ErrClaim
			}
		}
		return nil
	}
	pin := p.TerminalExperience
	if (status != "active" && status != "blocked") || p.Handler != "task-experience-v1" || p.Trigger != "source_changed" || p.MaxJobs != 1 || p.Job.Allowance.ModelTurns != 1 || p.Job.TaskID == "" || p.Job.OriginalRunID == "" || len(p.Contexts) != 0 || pin.SourceID == "" || len(pin.ObservationHash) != 64 {
		return ErrClaim
	}
	found := false
	for _, ref := range p.Job.Sources {
		found = found || ref.ID == pin.SourceID
	}
	if !found {
		return ErrClaim
	}
	if pin.Reconciled {
		if status != "active" {
			return ErrClaim
		}
		var current bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_observations o JOIN memory_experience_reconciled_attempts a USING(source_id)
WHERE o.source_id=$1 AND o.observation_hash=$2 AND o.owner_id=$3 AND o.task_id=$4 AND o.run_id=$5 AND o.result_id='' AND butler_memory_reconciled_attempt_current(o.source_id)
AND a.current_task_revision=$6 AND a.current_constraints_revision=$7
AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=o.session_key AND l.lease_id<>'' AND l.expires_at>clock_timestamp()))`, pin.SourceID, pin.ObservationHash, p.OwnerID, p.Job.TaskID, p.Job.OriginalRunID, p.TaskRevision, p.ConstraintsRevision).Scan(&current)
		if err != nil {
			return err
		}
		if !current {
			return ErrClaim
		}
		return nil
	}
	// This early task/policy observation must not acquire the input fence before
	// Catalog source locks. Full AccessTx/Finalize take source -> tool -> input
	// fences and repeat this metadata proof. Privacy deletion uses that order too.
	var current bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_experience_observations o JOIN memory_experience_terminal_attempts a USING(source_id) JOIN runs r ON r.run_id=o.run_id
WHERE o.source_id=$1 AND o.observation_hash=$2 AND o.owner_id=$3 AND o.task_id=$4 AND o.run_id=$5 AND o.result_id='' AND butler_memory_terminal_attempt_current(o.source_id)
AND r.task_revision=$6
AND NOT EXISTS(SELECT 1 FROM session_ownership l WHERE l.session_key=o.session_key AND l.lease_id<>'' AND l.expires_at>clock_timestamp()))`, pin.SourceID, pin.ObservationHash, p.OwnerID, p.Job.TaskID, p.Job.OriginalRunID, p.TaskRevision).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return ErrClaim
	}
	return nil
}
