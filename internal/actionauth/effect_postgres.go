package actionauth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type effectQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func effectIn(ctx context.Context, q effectQuerier, id string, lock bool) (EffectRecord, error) {
	var r EffectRecord
	var binding, policy string
	sql := `SELECT action_id,fingerprint,initial_binding::text,policy::text,state,idempotency_key,authorization_id,dispatch_outcome,created_at,updated_at FROM action_effects WHERE action_id=$1`
	if lock {
		sql += " FOR UPDATE"
	}
	err := q.QueryRow(ctx, sql, id).Scan(&r.ActionID, &r.Fingerprint, &binding, &policy, &r.State, &r.IdempotencyKey, &r.AuthorizationID, &r.DispatchOutcome, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(binding), &r.Binding); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(policy), &r.Policy); err != nil {
		return r, err
	}
	rows, err := q.Query(ctx, `SELECT evidence_id FROM action_effect_evidence WHERE action_id=$1 ORDER BY recorded_at,evidence_id`, id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	r.EvidenceRefs = []string{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return r, err
		}
		r.EvidenceRefs = append(r.EvidenceRefs, ref)
	}
	return r, rows.Err()
}
func (s *PostgresStore) GetEffect(ctx context.Context, id string) (EffectRecord, error) {
	return effectIn(ctx, s.pool, id, false)
}
func (s *PostgresStore) GetEffectEvidence(ctx context.Context, id string) (EffectEvidence, error) {
	var e EffectEvidence
	var data string
	err := s.pool.QueryRow(ctx, `SELECT evidence::text FROM action_effect_evidence WHERE evidence_id=$1`, id).Scan(&data)
	if err == nil {
		err = json.Unmarshal([]byte(data), &e)
	}
	return e, err
}

type RunEffect struct {
	ActionID     string      `json:"action_id"`
	EffectState  EffectState `json:"effect_state"`
	EvidenceRefs []string    `json:"evidence_refs"`
}

// GetRunEvidence exposes a typed snapshot for final-result coordination. It
// deliberately excludes secret-bearing resources and raw recipient responses.
func (s *PostgresStore) GetRunEvidence(ctx context.Context, runID string) ([]RunEffect, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT e.action_id,e.state FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.run_id=$1 ORDER BY e.action_id`, runID)
	if err != nil {
		return nil, err
	}
	result := []RunEffect{}
	for rows.Next() {
		var r RunEffect
		if err = rows.Scan(&r.ActionID, &r.EffectState); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range result {
		record, e := s.GetEffect(ctx, result[i].ActionID)
		if e != nil {
			return nil, e
		}
		result[i].EffectState = record.State
		result[i].EvidenceRefs = record.EvidenceRefs
	}
	return result, nil
}

func insertEvidence(ctx context.Context, tx pgx.Tx, e EffectEvidence) error {
	if e.EvidenceID == "" || e.ActionID == "" || e.SourceActionID == "" || e.SourceRunID == "" || e.SourceToolCallID == "" || e.ObservedAt.IsZero() || e.RecordedAt.IsZero() || e.ObservedAt.After(e.RecordedAt) || e.RecordedAt.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("invalid effect evidence provenance")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO action_effect_evidence(evidence_id,action_id,source_action_id,evidence,observed_at,recorded_at) VALUES($1,$2,$3,$4::jsonb,$5,$6)`, e.EvidenceID, e.ActionID, e.SourceActionID, string(data), e.ObservedAt, e.RecordedAt)
	return err
}

// The grant, journal, dispatch outcome and append-only evidence commit together.
// A database failure leaves executing, so recovery must observe rather than replay.
func (s *PostgresStore) RecordDispatchOutcome(ctx context.Context, id, outcome string, e EffectEvidence) (EffectRecord, error) {
	if outcome != "dispatch_completed" && outcome != "unknown" {
		return EffectRecord{}, fmt.Errorf("invalid dispatch outcome")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EffectRecord{}, err
	}
	defer tx.Rollback(ctx)
	g, err := grantInTx(ctx, tx, id)
	if err != nil {
		return EffectRecord{}, err
	}
	var effectID string
	err = tx.QueryRow(ctx, `SELECT effect_id FROM action_intents WHERE action_id=$1`, g.Binding.ActionID).Scan(&effectID)
	if err != nil {
		return EffectRecord{}, err
	}
	r, err := effectIn(ctx, tx, effectID, true)
	if err != nil {
		return r, err
	}
	if g.State != "inflight" || r.State != StateExecuting || r.AuthorizationID != id || e.ActionID != r.ActionID || e.SourceActionID != g.Binding.ActionID || e.SourceRunID != g.Binding.RunID || e.SourceToolCallID != g.Binding.ToolCallID || e.Source != "runtime_response" || e.Method != r.Policy.VerificationMethod {
		return r, fmt.Errorf("dispatch evidence does not match the executing intent")
	}
	if e.Verdict != StateUnknown && e.Verdict != StateConfirmed && (e.Verdict != StateAbsent || !browserReadAbsenceEvidence(g.Binding, r.Policy, outcome, e)) {
		return r, fmt.Errorf("dispatch cannot prove absence without owning pre-navigation read evidence")
	}
	if e.Verdict == StateConfirmed && (outcome != "dispatch_completed" || e.ResultDigest == "" || (r.Policy.VerificationMethod == "none") || (r.Policy.VerificationMethod == "external_receipt_v1" && e.ArtifactDigest == "")) {
		return r, fmt.Errorf("confirmed effect requires supported evidence")
	}
	if err = insertEvidence(ctx, tx, e); err != nil {
		return r, err
	}
	tag, err := tx.Exec(ctx, `UPDATE action_authorizations SET state=$2,finished_at=NOW() WHERE authorization_id=$1 AND state='inflight'`, id, outcome)
	if err != nil || tag.RowsAffected() != 1 {
		return r, fmt.Errorf("dispatch outcome cannot be recorded")
	}
	if _, err = tx.Exec(ctx, `UPDATE action_effect_dispatches SET outcome=$2,finished_at=NOW() WHERE authorization_id=$1 AND outcome='executing'`, id, outcome); err != nil {
		return r, err
	}
	if err = tx.QueryRow(ctx, `UPDATE action_effects SET state=$2,dispatch_outcome=$3,updated_at=NOW() WHERE action_id=$1 RETURNING updated_at`, r.ActionID, string(e.Verdict), outcome).Scan(&r.UpdatedAt); err != nil {
		return r, err
	}
	r.State, r.DispatchOutcome = e.Verdict, outcome
	r.EvidenceRefs = append(r.EvidenceRefs, e.EvidenceID)
	if s.budgets != nil {
		category := string(e.Verdict)
		resultDigest := e.NormalizedResultDigest
		if e.ErrorCategory != "" {
			category = e.ErrorCategory
			resultDigest = ""
		}
		if err = s.budgets.ObserveToolTx(ctx, tx, toolBudgetIdentity(g, r.ActionID), category, resultDigest); err != nil {
			return r, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return r, err
	}
	return r, nil
}

// Only a separately captured authorized read can reconcile an uncertain effect.
// Evidence, task/owner identity and the actual captured digest are rechecked in PG.
func (s *PostgresStore) ReconcileEffect(ctx context.Context, targetID, sourceID string, e EffectEvidence) (EffectRecord, error) {
	if targetID == sourceID {
		return EffectRecord{}, fmt.Errorf("independent observation is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EffectRecord{}, err
	}
	defer tx.Rollback(ctx)
	target, err := effectIn(ctx, tx, targetID, true)
	if err != nil {
		return target, err
	}
	source, err := effectIn(ctx, tx, sourceID, false)
	if err != nil {
		return target, err
	}
	if (target.State != StateUnknown && target.State != StateExecuting) || source.State != StateConfirmed || source.Binding.Effect != EffectRead || source.Binding.ToolName != "http.request" || target.Policy.VerificationMethod != "external_receipt_v1" || e.Source != "independent_lookup" || e.Method != "external_receipt_v1" || e.ActionID != targetID || source.Binding.OwnerID != target.Binding.OwnerID || source.Binding.TaskID != target.Binding.TaskID || (e.Verdict != StateConfirmed && e.Verdict != StateAbsent) || e.ResultDigest == "" || (e.Verdict == StateConfirmed && e.ArtifactDigest == "") {
		return target, fmt.Errorf("independent evidence cannot reconcile this intent")
	}
	expected := verificationResource(target)
	var data string
	err = tx.QueryRow(ctx, `SELECT binding::text FROM action_intents WHERE action_id=$1 AND effect_id=$2`, e.SourceActionID, sourceID).Scan(&data)
	if err != nil {
		return target, err
	}
	var binding ActionBinding
	if json.Unmarshal([]byte(data), &binding) != nil || binding.Resource != expected || binding.RunID != e.SourceRunID || binding.ToolCallID != e.SourceToolCallID || binding.Effect != EffectRead {
		return target, fmt.Errorf("observation resource or provenance changed")
	}
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effect_evidence WHERE action_id=$1 AND source_action_id=$2 AND evidence->>'source'='runtime_response' AND evidence->>'verdict'='effect_confirmed' AND evidence->>'source_run_id'=$3 AND evidence->>'source_tool_call_id'=$4 AND evidence->>'result_digest'=$5 AND observed_at=$6)`, sourceID, e.SourceActionID, e.SourceRunID, e.SourceToolCallID, e.ResultDigest, e.ObservedAt).Scan(&exists)
	if err != nil || !exists {
		return target, fmt.Errorf("observation was not independently captured")
	}
	if err = insertEvidence(ctx, tx, e); err != nil {
		return target, err
	}
	if err = tx.QueryRow(ctx, `UPDATE action_effects SET state=$2,dispatch_outcome=CASE WHEN dispatch_outcome='executing' THEN 'unknown' ELSE dispatch_outcome END,updated_at=NOW() WHERE action_id=$1 RETURNING updated_at,dispatch_outcome`, targetID, string(e.Verdict)).Scan(&target.UpdatedAt, &target.DispatchOutcome); err != nil {
		return target, err
	}
	if _, err = tx.Exec(ctx, `UPDATE action_authorizations SET state='unknown',finished_at=COALESCE(finished_at,NOW()) WHERE authorization_id=$1 AND state='inflight'`, target.AuthorizationID); err != nil {
		return target, err
	}
	if _, err = tx.Exec(ctx, `UPDATE action_effect_dispatches SET outcome='unknown',finished_at=COALESCE(finished_at,NOW()) WHERE authorization_id=$1 AND outcome='executing'`, target.AuthorizationID); err != nil {
		return target, err
	}
	target.State = e.Verdict
	target.EvidenceRefs = append(target.EvidenceRefs, e.EvidenceID)
	if err = tx.Commit(ctx); err != nil {
		return target, err
	}
	return target, nil
}

func (s *PostgresStore) MarkInterruptedEffect(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	r, err := effectIn(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if r.State != StateExecuting {
		return fmt.Errorf("only an interrupted executing effect can become unknown")
	}
	if _, err = tx.Exec(ctx, `UPDATE action_effects SET state='effect_unknown',dispatch_outcome='unknown',updated_at=NOW() WHERE action_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE action_authorizations SET state='unknown',finished_at=NOW() WHERE authorization_id=$1 AND state='inflight'`, r.AuthorizationID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE action_effect_dispatches SET outcome='unknown',finished_at=NOW() WHERE authorization_id=$1 AND outcome='executing'`, r.AuthorizationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
