package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

type delegationReportEvidence struct {
	report       delegation.Report
	final        completion.FinalRecord
	refs         map[string]delegation.EvidenceRef
	confirmed    map[string]bool
	reasons      map[string]bool
	actions      []delegation.ActionObservation
	used         delegation.Limits
	reservations []string
}

func (g *DelegationGuards) CheckReportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result, report delegation.SourceRef) error {
	_, err := g.reportTx(ctx, tx, parent, child, result, report, nil)
	return err
}

func (g *DelegationGuards) reportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result, ref delegation.SourceRef, extra []delegation.SourceRef) (delegationReportEvidence, error) {
	fail := func() (delegationReportEvidence, error) { return delegationReportEvidence{}, delegation.ErrDenied }
	c, err := delegation.LoadTx(ctx, tx, child.TaskID)
	if err != nil || c == nil {
		return fail()
	}
	if err = g.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return delegationReportEvidence{}, err
	}
	var final completion.FinalRecord
	var effects, limitations []byte
	// The terminal run identity and immutable final row are observations, not a
	// new worker lease. Do not lock a historical run after task/source locks.
	err = tx.QueryRow(ctx, `SELECT f.result_id,f.run_id,f.session_key,f.task_id,f.task_revision,f.constraints_revision,f.artifact_id,f.message_id,f.content_hash,f.answer_state,f.effects_json,f.limitations_json,f.privacy_redacted FROM run_final_results f JOIN runs r USING(run_id) WHERE f.run_id=$1 AND r.current_state='completed' AND r.task_id=$2 AND r.task_revision=$3 AND r.session_key=$4`, result.RunID, child.TaskID, child.Revision, child.SessionKey).Scan(&final.ResultID, &final.RunID, &final.SessionKey, &final.TaskID, &final.TaskRevision, &final.ConstraintsRevision, &final.ArtifactID, &final.MessageID, &final.ContentHash, &final.AnswerState, &effects, &limitations, &final.PrivacyRedacted)
	if err != nil || final.PrivacyRedacted || final.TaskID != child.TaskID || final.TaskRevision != child.Revision || final.ConstraintsRevision != child.ConstraintsRevision || result.TaskID != child.TaskID || result.TaskRevision != child.Revision || json.Unmarshal(effects, &final.Effects) != nil || json.Unmarshal(limitations, &final.Limitations) != nil {
		return fail()
	}
	var artifactText, messageText string
	err = tx.QueryRow(ctx, `SELECT a.content_text,m.content FROM artifacts a JOIN messages m ON m.message_id=$2 WHERE a.artifact_id=$1 AND a.run_id=$3 AND m.run_id=$3 AND a.session_key=$4 AND m.session_key=$4 AND a.artifact_type='assistant_final' AND m.role='assistant' AND NOT a.privacy_redacted AND NOT m.privacy_redacted`, final.ArtifactID, final.MessageID, final.RunID, child.SessionKey).Scan(&artifactText, &messageText)
	checksum := sha256.Sum256([]byte(artifactText))
	if err != nil || artifactText != messageText || hex.EncodeToString(checksum[:]) != final.ContentHash {
		return fail()
	}
	// Read the actual result again, rather than accepting a caller's forged
	// Result value or a report's claimed verification/criteria/evidence.
	var stored task.Result
	var evidence, criteria []byte
	err = tx.QueryRow(ctx, `SELECT result_id,task_id,task_revision,run_id,verdict,verifier,evidence_refs_json,verified_criteria_json FROM task_results WHERE result_id=$1 AND NOT privacy_redacted FOR SHARE`, result.ResultID).Scan(&stored.ResultID, &stored.TaskID, &stored.TaskRevision, &stored.RunID, &stored.Verdict, &stored.Verifier, &evidence, &criteria)
	if err != nil || json.Unmarshal(evidence, &stored.EvidenceRefs) != nil || json.Unmarshal(criteria, &stored.VerifiedCriteria) != nil || stored.TaskID != child.TaskID || stored.TaskRevision != child.Revision || stored.RunID != final.RunID || stored.Verdict != result.Verdict || stored.Verifier != result.Verifier || !equalStrings(stored.EvidenceRefs, result.EvidenceRefs) || !equalStrings(stored.VerifiedCriteria, result.VerifiedCriteria) {
		return fail()
	}
	if !delegation.ValidSource(ref) {
		return fail()
	}
	rows, err := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id`, final.RunID)
	if err != nil {
		return delegationReportEvidence{}, err
	}
	pins := []catalog.SourceRef{}
	for rows.Next() {
		var p catalog.SourceRef
		if err = rows.Scan(&p.ID, &p.Revision, &p.AccessRevision, &p.Hash); err != nil {
			break
		}
		pins = append(pins, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return delegationReportEvidence{}, err
	}
	// The final-message root retains the actual run's complete original pins.
	var finalSource catalog.SourceRef
	err = tx.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1 AND s.owner_id=$2 AND s.status='active'`, "message:"+final.MessageID, child.OwnerID).Scan(&finalSource.ID, &finalSource.Revision, &finalSource.AccessRevision, &finalSource.Hash)
	if err != nil {
		return fail()
	}
	roots := append(refsForDelegation(*c), pins...)
	roots = append(roots, finalSource)
	allowed, err := g.sources.CanonicalSourcesTx(ctx, tx, roots)
	if err != nil {
		return delegationReportEvidence{}, err
	}
	graph, err := g.sources.CanonicalSourcesTx(ctx, tx, catalogRefs([]delegation.SourceRef{ref}))
	if err != nil {
		return delegationReportEvidence{}, err
	}
	byID := map[string]catalog.SourceRef{}
	for _, p := range graph {
		byID[p.ID] = p
	}
	for _, p := range roots {
		a, ok := byID[p.ID]
		if !ok || a.Revision != p.Revision {
			return fail()
		}
	}
	permitted := map[string]bool{ref.ID: true}
	for _, p := range allowed {
		permitted[p.ID] = true
	}
	for _, p := range graph {
		if !permitted[p.ID] {
			return fail()
		}
	}
	for _, p := range extra {
		if !permitted[p.ID] || p.ID == ref.ID {
			return fail()
		}
	}
	checker := delegationReportAccess{guard: g, parent: parent, child: child, contract: *c, requested: append(catalogRefs([]delegation.SourceRef{ref}), catalogRefs(extra)...), pins: roots}
	accessCtx := catalog.WithAccessChecker(ctx, checker)
	sources, err := g.sources.AccessTx(accessCtx, tx, checker.requested, "delegation_report")
	if err != nil {
		return delegationReportEvidence{}, err
	}
	var source catalog.Source
	for _, s := range sources {
		if s.ID == ref.ID {
			source = s
		}
	}
	if source.OwnerID != child.OwnerID || !(source.ScopeType == "task" && source.ScopeID == child.TaskID || source.ScopeType == "session" && source.ScopeID == child.SessionKey) {
		return fail()
	}
	parsed, err := delegation.ParseReport([]byte(source.Body))
	if err != nil {
		return delegationReportEvidence{}, err
	}
	if parsed.TaskID != child.TaskID || parsed.TaskRevision != child.Revision || parsed.ContractRevision != c.Revision || parsed.ResultID != stored.ResultID {
		return fail()
	}
	actual := delegationReportEvidence{report: parsed, final: final, refs: map[string]delegation.EvidenceRef{}, confirmed: map[string]bool{}, reasons: map[string]bool{}}
	for _, p := range append(allowed, catalogRefs(extra)...) {
		// Actual source pins carry all four fields. Ancestors without a supplied
		// hash are privacy dependencies, never hashless report evidence.
		if p.Hash != "" {
			actual.refs["source:"+p.ID] = delegation.EvidenceRef{Kind: "source", ID: p.ID, Revision: p.Revision, Hash: p.Hash}
		}
	}
	actual.refs["result:"+stored.ResultID] = delegation.EvidenceRef{Kind: "result", ID: stored.ResultID, Revision: child.Revision, Hash: final.ContentHash}
	actual.refs["artifact:"+final.ArtifactID] = delegation.EvidenceRef{Kind: "artifact", ID: final.ArtifactID, Hash: final.ContentHash}
	if stored.Verdict != "verified" || stored.Verifier != "owner" {
		actual.reasons["verification_missing"] = true
	}
	if final.AnswerState == "partial" {
		actual.reasons["execution_failed"] = true
	}
	if err = loadDelegationActions(ctx, tx, child, final, &actual); err != nil {
		return delegationReportEvidence{}, err
	}
	if err = loadDelegationBudget(ctx, tx, child, &actual); err != nil {
		return delegationReportEvidence{}, err
	}
	if !equalDelegationActions(parsed.Actions, actual.actions) || parsed.Budget.Used != actual.used || !equalStrings(parsed.Budget.ReservationIDs, actual.reservations) {
		return fail()
	}
	for _, r := range append(append([]delegation.EvidenceRef(nil), parsed.Evidence...), parsed.Accomplished...) {
		if !actual.validRef(r) {
			return fail()
		}
	}
	for _, r := range parsed.Accomplished {
		if r.Kind == "action" && !actual.confirmed[r.ID] || r.Kind == "result" && actual.reasons["verification_missing"] {
			return fail()
		}
	}
	seenReasons := map[string]bool{}
	for _, r := range append(append([]delegation.Reason(nil), parsed.Uncertainty...), parsed.Blockers...) {
		if !actual.reasons[r.Kind] || r.Ref != nil && !actual.validRef(*r.Ref) {
			return fail()
		}
		seenReasons[r.Kind] = true
	}
	for reason := range actual.reasons {
		if !seenReasons[reason] {
			return fail()
		}
	}
	// Recheck DB time after source/effect/budget waits. All mutable authority
	// rows are already held; this does not change the established lock order.
	if err = g.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return delegationReportEvidence{}, err
	}
	return actual, nil
}

type delegationReportAccess struct {
	guard           *DelegationGuards
	parent, child   task.Record
	contract        delegation.Contract
	requested, pins []catalog.SourceRef
}

func (c delegationReportAccess) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	if purpose != "delegation_report" || !reflect.DeepEqual(refs, c.requested) {
		return "", delegation.ErrDenied
	}
	if err := currentDelegationTask(ctx, tx, c.parent); err != nil {
		return "", err
	}
	if err := currentDelegationTask(ctx, tx, c.child); err != nil {
		return "", err
	}
	if err := c.guard.sources.CheckDelegationSourcesTx(ctx, tx, c.child.TaskID, refsForDelegation(c.contract), "memory_read"); err != nil {
		return "", err
	}
	return c.child.OwnerID, nil
}
func (c delegationReportAccess) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) error {
	_, err := c.CheckMemoryAccessTx(ctx, tx, refs, purpose)
	return err
}
func (c delegationReportAccess) PinnedMemorySources() []catalog.SourceRef {
	return append([]catalog.SourceRef(nil), c.pins...)
}

func (a delegationReportEvidence) validRef(r delegation.EvidenceRef) bool {
	want, ok := a.refs[r.Kind+":"+r.ID]
	return ok && r == want
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func (g *DelegationGuards) CheckDecisionTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result, d task.DelegationDecision) error {
	a, err := g.reportTx(ctx, tx, parent, child, result, d.Report, d.Evidence)
	if err != nil {
		return err
	}
	if len(d.Evidence) == 0 {
		return delegation.ErrDenied
	}
	// A reject may retain unknown/partial observations. Accept requires the
	// independently persisted owner verification for this exact result/run and
	// criteria, plus current source evidence explicitly listed by that verifier.
	if d.Accept {
		if result.Verdict != "verified" || result.Verifier != "owner" || a.final.AnswerState != "final" || len(a.reasons) != 0 || !equalStrings(result.VerifiedCriteria, child.CompletionCriteria) {
			return delegation.ErrDenied
		}
		seen := map[string]bool{}
		for _, r := range d.Evidence {
			if seen[r.ID] || !hasExact(result.EvidenceRefs, DelegationSourceEvidence(r)) {
				return delegation.ErrDenied
			}
			seen[r.ID] = true
		}
		for _, action := range a.report.Actions {
			for _, ref := range append([]delegation.EvidenceRef{a.refs["action:"+action.ActionID]}, action.Evidence...) {
				if !hasExact(result.EvidenceRefs, DelegationRecordEvidence(ref)) {
					return delegation.ErrDenied
				}
			}
		}
	}
	return nil
}

// These content-free version/hash pins are written only by an independently
// authenticated owner verifier. Existing prose/unversioned result evidence is
// retained as history but cannot validate a new delegated acceptance.
func DelegationSourceEvidence(r delegation.SourceRef) string {
	return fmt.Sprintf("catalog:%s:%d:%d:%s", r.ID, r.Revision, r.AccessRevision, r.Hash)
}
func DelegationRecordEvidence(r delegation.EvidenceRef) string {
	return fmt.Sprintf("record:%s:%s:%d:%s", r.Kind, r.ID, r.Revision, r.Hash)
}
