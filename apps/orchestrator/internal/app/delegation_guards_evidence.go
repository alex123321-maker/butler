package app

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/jackc/pgx/v5"
)

// These are read-only adapters over owning stores' actual records on the
// manager transaction. They never write journal, result or reservation state.
func loadDelegationActions(ctx context.Context, tx pgx.Tx, child task.Record, final completion.FinalRecord, a *delegationReportEvidence) error {
	type observed struct {
		binding         actionauth.ActionBinding
		effectID, state string
	}
	rows, err := tx.Query(ctx, `SELECT i.action_id,i.action_hash,i.binding,i.effect_id,e.state FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id=$1 ORDER BY e.action_id,i.action_id FOR SHARE OF i,e`, final.RunID)
	if err != nil {
		return err
	}
	var actual []observed
	for rows.Next() {
		var id, hash string
		var raw []byte
		var item observed
		if err = rows.Scan(&id, &hash, &raw, &item.effectID, &item.state); err != nil {
			break
		}
		if json.Unmarshal(raw, &item.binding) != nil || item.binding.ActionID != id || item.binding.ActionHash != hash || item.binding.RunID != final.RunID || item.binding.TaskID != child.TaskID || item.binding.TaskRevision != child.Revision || item.binding.ConstraintsRevision != child.ConstraintsRevision || item.binding.OwnerID != child.OwnerID {
			err = delegation.ErrDenied
			break
		}
		actual = append(actual, item)
		if len(actual) > 64 {
			err = delegation.ErrDenied
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	snapshots := map[string]completion.Effect{}
	for _, s := range final.Effects {
		if _, ok := snapshots[s.ActionID]; ok {
			return delegation.ErrDenied
		}
		snapshots[s.ActionID] = s
	}
	seenEffects := map[string]bool{}
	calls := map[string]bool{}
	for _, item := range actual {
		b := item.binding
		seenEffects[item.effectID] = true
		calls[b.ToolCallID] = true
		a.refs["action:"+b.ActionID] = delegation.EvidenceRef{Kind: "action", ID: b.ActionID, Hash: b.ActionHash}
		if item.state == string(actionauth.StateConfirmed) {
			a.confirmed[b.ActionID] = true
		}
		switch actionauth.EffectState(item.state) {
		case actionauth.StateUnknown, actionauth.StateExecuting:
			a.reasons["effect_unknown"] = true
		case actionauth.StatePrepared, actionauth.StateAllowed:
			a.reasons["verification_missing"] = true
		case actionauth.StateAbsent:
			a.reasons["execution_failed"] = true
		}
		rows, err = tx.Query(ctx, `SELECT evidence_id,source_action_id,evidence FROM action_effect_evidence WHERE action_id=$1 ORDER BY evidence_id`, item.effectID)
		if err != nil {
			return err
		}
		var ids []string
		var proofs []delegation.EvidenceRef
		for rows.Next() {
			var id, sourceID string
			var raw []byte
			var e actionauth.EffectEvidence
			if err = rows.Scan(&id, &sourceID, &raw); err != nil {
				break
			}
			if json.Unmarshal(raw, &e) != nil || e.EvidenceID != id || e.ActionID != item.effectID || e.SourceActionID != sourceID || e.SourceRunID == "" || e.SourceToolCallID == "" || e.ObservedAt.IsZero() || e.RecordedAt.Before(e.ObservedAt) {
				err = delegation.ErrDenied
				break
			}
			// A permanent effect may contain earlier-attempt reconciliation. Only
			// this exact child's execution can be claimed in this run's report.
			if e.SourceActionID == b.ActionID {
				if e.SourceRunID != b.RunID || e.SourceToolCallID != b.ToolCallID {
					err = delegation.ErrDenied
					break
				}
				proof := delegation.EvidenceRef{Kind: "action", ID: id, Hash: e.ResultDigest}
				proofs = append(proofs, proof)
				a.refs["action:"+id] = proof
				if e.Verdict == actionauth.StateConfirmed {
					a.confirmed[id] = true
				}
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		snapshot, ok := snapshots[item.effectID]
		if !ok || snapshot.EffectState != item.state || !equalStrings(snapshot.EvidenceRefs, ids) {
			return delegation.ErrDenied
		}
		a.actions = append(a.actions, delegation.ActionObservation{ActionID: b.ActionID, EffectID: item.effectID, State: item.state, Evidence: proofs})
		if item.state == string(actionauth.StateConfirmed) && len(proofs) == 0 {
			return delegation.ErrDenied
		}
	}
	if len(seenEffects) != len(snapshots) {
		return delegation.ErrDenied
	}
	rows, err = tx.Query(ctx, `SELECT tool_call_id,status FROM tool_calls WHERE run_id=$1 ORDER BY tool_call_id`, final.RunID)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var id, state string
		if err = rows.Scan(&id, &state); err != nil {
			break
		}
		count++
		if !calls[id] {
			a.reasons["verification_missing"] = true
		}
		if state != "completed" {
			a.reasons["execution_failed"] = true
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if count > 64 {
		return delegation.ErrDenied
	}
	return nil
}

func equalEvidence(a, b []delegation.EvidenceRef) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]delegation.EvidenceRef(nil), a...), append([]delegation.EvidenceRef(nil), b...)
	sort.Slice(x, func(i, j int) bool { return x[i].ID < x[j].ID })
	sort.Slice(y, func(i, j int) bool { return y[i].ID < y[j].ID })
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func loadDelegationBudget(ctx context.Context, tx pgx.Tx, child task.Record, a *delegationReportEvidence) error {
	scopeID := budget.ScopeID("delegation", child.OwnerID, child.TaskID)
	var usedRaw []byte
	err := tx.QueryRow(ctx, `SELECT used_json FROM execution_budget_scopes WHERE scope_id=$1 AND kind='delegation' AND owner_id=$2 AND subject_id=$3 FOR SHARE`, scopeID, child.OwnerID, child.TaskID).Scan(&usedRaw)
	var used delegation.Limits
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil && json.Unmarshal(usedRaw, &used) != nil {
		return delegation.ErrDenied
	}
	rows, err := tx.Query(ctx, `SELECT reservation_id,fingerprint,request_json FROM execution_budget_reservations WHERE owner_id=$1 AND request_json->'scope_ids' ? $2 ORDER BY reservation_id`, child.OwnerID, scopeID)
	if err != nil {
		return err
	}
	ids := []string{}
	var summed [8]int64
	for rows.Next() {
		var id, fingerprint string
		var raw []byte
		var r budget.Request
		if err = rows.Scan(&id, &fingerprint, &raw); err != nil {
			break
		}
		if json.Unmarshal(raw, &r) != nil || r.OwnerID != child.OwnerID || !r.CostKnown || !hasExact(r.ScopeIDs, scopeID) {
			err = delegation.ErrDenied
			break
		}
		for i, v := range [8]int64{r.Charge.ModelTurns, r.Charge.Tokens, r.Charge.CostMicros, r.Charge.ExternalCalls, r.Charge.Children, r.Charge.ActiveMillis, r.Charge.EgressBytes, r.Charge.ProviderInputBytes} {
			if v < 0 || v > math.MaxInt64-summed[i] {
				err = delegation.ErrDenied
				break
			}
			summed[i] += v
		}
		if err != nil {
			break
		}
		ids = append(ids, id)
		a.refs["reservation:"+id] = delegation.EvidenceRef{Kind: "reservation", ID: id, Hash: fingerprint}
		if len(ids) > 128 {
			err = delegation.ErrDenied
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if summed != delegationUnits(used) {
		return delegation.ErrDenied
	}
	a.used, a.reservations = used, ids
	// All actual child-run inference and dispatched effect reservations must
	// be included; a forged/missing branch scope cannot make usage disappear.
	rows, err = tx.Query(ctx, `SELECT reservation_id FROM execution_model_attempts WHERE run_id=$1 UNION SELECT r.reservation_id FROM execution_budget_reservations r JOIN action_intents i ON r.operation_id='effect:'||i.effect_id WHERE i.run_id=$1 AND r.owner_id=$2`, a.final.RunID, child.OwnerID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		if !hasExact(ids, id) {
			err = delegation.ErrDenied
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	return err
}
