package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// AcceptVerified accepts an already collected unverified final after a separate
// authenticated owner verification. The old result/report remain immutable;
// a fresh kernel report and the accepted pointer commit together. It cannot
// replace a decision, switch runs, complete a task or admit new execution.
func (m *DelegationManager) AcceptVerified(ctx context.Context, parentID, childID string, previousReport delegation.SourceRef, verifiedResultID string, evidence []delegation.SourceRef) (DelegationRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	actor, owner := owneridentity.FromContext(ctx)
	_, worker := ownership.TokenFromContext(ctx)
	if !owner || worker {
		return DelegationRecord{}, ErrUnauthorized
	}
	if !delegation.ValidSource(previousReport) || verifiedResultID == "" || len(evidence) < 1 || len(evidence) > 32 {
		return DelegationRecord{}, ErrInvalid
	}
	evidence = append([]delegation.SourceRef(nil), evidence...)
	for _, ref := range evidence {
		if !delegation.ValidSource(ref) {
			return DelegationRecord{}, ErrInvalid
		}
	}
	creator, ok := m.guard.(DelegationReportCreator)
	if !ok {
		return DelegationRecord{}, delegation.ErrDenied
	}
	tx, _, parent, err := m.begin(ctx, parentID)
	if err != nil {
		return DelegationRecord{}, err
	}
	defer delegationRollback(tx)
	if actor.OwnerID != parent.OwnerID || (actor.SessionKey != "" && actor.SessionKey != parent.SessionKey) {
		return DelegationRecord{}, ErrUnauthorized
	}
	child, err := m.store.GetTx(ctx, tx, childID)
	if err != nil {
		return DelegationRecord{}, err
	}
	v, err := m.loadTx(ctx, tx, childID)
	if err != nil {
		return v, err
	}
	if v.Contract.State != "result_ready" || v.Decision != nil || v.Report == nil || *v.Report != previousReport || v.ResultID == "" || v.ResultID == verifiedResultID {
		return v, ErrConflict
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	old, err := m.resultTx(ctx, tx, child, v.ResultID)
	if err != nil {
		return v, err
	}
	if old.Verdict != "unverified" {
		return v, ErrConflict
	}
	// Deletion/staleness of the original collected graph denies promotion too.
	if err = m.guard.CheckReportTx(ctx, tx, parent, child, old, previousReport); err != nil {
		return v, err
	}
	verified, err := m.resultTx(ctx, tx, child, verifiedResultID)
	if err != nil {
		return v, err
	}
	if verified.RunID != old.RunID || verified.Verdict != "verified" || verified.Verifier != "owner" {
		return v, ErrInvalid
	}
	report, err := creator.CreateReportTx(ctx, tx, parent, child, verified)
	if err != nil || !delegation.ValidSource(report) {
		if err == nil {
			err = delegation.ErrDenied
		}
		return v, err
	}
	decision := DelegationDecision{Accept: true, Report: report, Evidence: evidence}
	if err = m.guard.CheckDecisionTx(ctx, tx, parent, child, verified, decision); err != nil {
		return v, err
	}
	if err = delegationCallerTx(ctx, tx, parent); err != nil {
		return v, err
	}
	if err = currentContractTx(ctx, tx, parent, child, v.Contract); err != nil {
		return v, err
	}
	stored, err := json.Marshal(delegationEvidence{Report: &report, Decision: &decision, PreviousReport: &previousReport, PreviousResultID: old.ResultID})
	if err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `UPDATE task_delegations SET state='accepted',result_id=$2,decision_evidence_json=$3::jsonb,updated_at=clock_timestamp() WHERE task_id=$1`, childID, verifiedResultID, stored); err != nil {
		return v, err
	}
	v.Contract.State, v.ResultID, v.Report, v.Decision = "accepted", verifiedResultID, &report, &decision
	return v, tx.Commit(ctx)
}
