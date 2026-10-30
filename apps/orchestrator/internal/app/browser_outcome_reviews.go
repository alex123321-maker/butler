package app

import (
	"context"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runstore "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type browserOutcomeKernel struct {
	Outcomes *actionauth.OwnerOutcomes
	Inputs   *durableinput.Store
}

func newBrowserOutcomeKernel(pool *pgxpool.Pool, inputs *durableinput.Store) *browserOutcomeKernel {
	k := &browserOutcomeKernel{Outcomes: &actionauth.OwnerOutcomes{Pool: pool}, Inputs: inputs}
	k.Outcomes.AfterDecisionTx = k.readyTx
	return k
}

func (k *browserOutcomeKernel) SuspendUnknown(ctx context.Context, runID, effectID, readCall string) (string, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return "", actionauth.ErrOwnerOutcome
	}
	tx, err := k.Outcomes.Pool.Begin(ctx)
	if err != nil {
		return "", actionauth.ErrOwnerOutcome
	}
	defer tx.Rollback(ctx)
	row, err := k.Outcomes.RequestTx(ctx, tx, effectID, readCall, false)
	if err != nil {
		return "", err
	}
	if task.WaitInteractionTx(ctx, tx, row.TaskID, row.TaskRevision, row.ConstraintsRevision) != nil || k.Inputs.SuspendPrivateInteractionTx(ctx, tx, runID, row.ID, "effect_review") != nil || runstore.SuspendBrowserOutcomeTx(ctx, tx, runID) != nil {
		return "", actionauth.ErrOwnerOutcome
	}
	if tx.Commit(ctx) != nil {
		return "", actionauth.ErrOwnerOutcomePending
	}
	return row.ID, nil
}

func (k *browserOutcomeKernel) readyTx(ctx context.Context, tx pgx.Tx, row actionauth.OwnerOutcomeReview) error {
	if row.SourceKind != "live_frozen" {
		return nil
	}
	var paused bool
	if row.TaskID != "" {
		if tx.QueryRow(ctx, `SELECT status='paused' FROM tasks WHERE task_id=$1 FOR SHARE`, row.TaskID).Scan(&paused) != nil {
			return actionauth.ErrOwnerOutcome
		}
	}
	// A factual response does not renew Input/execution deadlines. A late report
	// stays unavailable; closing expired waits is separate from authorizing work.
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='interaction_ready',status=CASE WHEN $3 THEN 'blocked' ELSE 'queued' END,expires_at=CASE WHEN $4 THEN GREATEST(expires_at,clock_timestamp()+interval '1 minute') ELSE expires_at END,not_before=clock_timestamp(),updated_at=clock_timestamp() WHERE run_id=$1 AND checkpoint='awaiting_interaction' AND checkpoint_json->'interaction'->>'interaction_id'=$2 AND checkpoint_json->'interaction'->>'kind'='effect_review' AND status IN ('waiting_for_user','blocked') AND claim_id='' AND (expires_at>clock_timestamp() OR $4)`, row.RunID, row.ID, paused, row.State == "unresolved")
	if err != nil || (tag.RowsAffected() != 1 && row.State != "unresolved") {
		return actionauth.ErrOwnerOutcome
	}
	return nil
}

func (k *browserOutcomeKernel) ResumeOutcome(ctx context.Context, runID, id string) (actionauth.OwnerOutcomeResolution, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	tx, err := k.Outcomes.Pool.Begin(ctx)
	if err != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	defer tx.Rollback(ctx)
	if checkCredentialLease(ctx, tx, token) != nil || k.Inputs.CheckInteractionClaimTx(ctx, tx, runID, id) != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	var owner, taskID, effect, call, state, fingerprint, evidence string
	var rev, cr int64
	err = tx.QueryRow(ctx, `SELECT owner_id,task_id,task_revision,constraints_revision,effect_id,tool_call_id,state,fingerprint,evidence_id FROM browser_owner_outcome_reviews WHERE review_id=$1 AND run_id=$2 AND session_key=$3 AND source_kind='live_frozen' AND expires_at>clock_timestamp() FOR SHARE`, id, runID, token.SessionKey).Scan(&owner, &taskID, &rev, &cr, &effect, &call, &state, &fingerprint, &evidence)
	if err != nil || (state != "effect_confirmed" && state != "effect_absent") || evidence == "" {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	var valid bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects e JOIN action_effect_evidence v ON v.action_id=e.action_id WHERE e.action_id=$1 AND e.fingerprint=$2 AND e.state=$3 AND v.evidence_id=$4 AND v.evidence->>'source'='authenticated_owner' AND v.evidence->>'method'='owner_outcome_attestation_v1' AND v.evidence->>'verdict'=$3)`, effect, fingerprint, state, evidence).Scan(&valid) != nil || !valid {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	var uncertain bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects e JOIN action_intents a ON a.effect_id=e.action_id WHERE a.run_id=$1 AND e.state IN ('executing','effect_unknown') AND a.binding->>'effect'<>'read')`, runID).Scan(&uncertain) != nil || uncertain {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	if task.ResumeInteractionTx(ctx, tx, taskID, rev, cr) != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	if taskID != "" {
		if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, time.Now()); err != nil {
			return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
		}
	}
	if tx.Commit(ctx) != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	return actionauth.OwnerOutcomeResolution{ReviewID: id, EffectID: effect, ToolCallID: call, State: actionauth.EffectState(state), EvidenceRefs: []string{evidence}, Source: "authenticated_owner_report"}, nil
}

func (k *browserOutcomeKernel) ReadResolvedOutcome(ctx context.Context, runID, call string) (actionauth.OwnerOutcomeResolution, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	ctx, tx, err := ownership.BeginTransaction(ctx, k.Outcomes.Pool)
	if err != nil {
		return actionauth.OwnerOutcomeResolution{}, err
	}
	defer tx.Rollback(ctx)
	var r actionauth.OwnerOutcomeResolution
	var evidence string
	err = tx.QueryRow(ctx, `SELECT o.review_id,o.effect_id,o.tool_call_id,o.state,o.evidence_id FROM browser_owner_outcome_reviews o JOIN action_effects e ON e.action_id=o.effect_id JOIN action_effect_evidence v ON v.evidence_id=o.evidence_id AND v.action_id=e.action_id JOIN sessions s ON s.session_key=o.session_key AND s.user_id=o.owner_id WHERE o.run_id=$1 AND o.session_key=$2 AND o.tool_call_id=$3 AND o.state IN ('effect_confirmed','effect_absent') AND e.state=o.state AND e.fingerprint=o.fingerprint AND v.evidence->>'method'='owner_outcome_attestation_v1' AND v.evidence->>'source'='authenticated_owner' AND v.evidence->>'verdict'=o.state FOR SHARE OF o,e,v`, runID, token.SessionKey, call).Scan(&r.ReviewID, &r.EffectID, &r.ToolCallID, &r.State, &evidence)
	if err != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	r.EvidenceRefs = []string{evidence}
	r.Source = "authenticated_owner_report"
	if tx.Commit(ctx) != nil {
		return actionauth.OwnerOutcomeResolution{}, actionauth.ErrOwnerOutcome
	}
	return r, nil
}
