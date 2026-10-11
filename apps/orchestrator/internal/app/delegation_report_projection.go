package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

type delegationDerivedCatalog interface {
	CreateDerivedTx(context.Context, pgx.Tx, []catalog.SourceRef, string, string, string, json.RawMessage) (catalog.Source, error)
}

var _ task.DelegationReportCreator = (*DelegationGuards)(nil)

// CreateReportTx derives observations from owning records. There is no model
// verdict, prose or caller-selected action/evidence list in this write path.
// Creating a fresh derived source and collection commit on the same transaction.
func (g *DelegationGuards) CreateReportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result) (delegation.SourceRef, error) {
	fail := func() (delegation.SourceRef, error) { return delegation.SourceRef{}, delegation.ErrDenied }
	writer, ok := g.sources.(delegationDerivedCatalog)
	if !ok {
		return fail()
	}
	c, err := delegation.LoadTx(ctx, tx, child.TaskID)
	if err != nil || c == nil {
		return fail()
	}
	if err = g.CheckLifecycleTx(ctx, tx, parent, child, *c); err != nil {
		return delegation.SourceRef{}, err
	}
	final, finalSource, pins, err := g.projectionFinalTx(ctx, tx, child, result)
	if err != nil {
		return delegation.SourceRef{}, err
	}
	a := delegationReportEvidence{final: final, refs: map[string]delegation.EvidenceRef{}, confirmed: map[string]bool{}, reasons: map[string]bool{}}
	if result.Verdict != "verified" || result.Verifier != "owner" {
		a.reasons["verification_missing"] = true
	}
	if final.AnswerState == "partial" {
		a.reasons["execution_failed"] = true
	}
	if err = loadDelegationActions(ctx, tx, child, final, &a); err != nil {
		return delegation.SourceRef{}, err
	}
	if err = loadDelegationBudget(ctx, tx, child, &a); err != nil {
		return delegation.SourceRef{}, err
	}
	r := delegation.Report{TaskID: child.TaskID, TaskRevision: child.Revision, ContractRevision: c.Revision, ResultID: result.ResultID, Actions: a.actions,
		Evidence: []delegation.EvidenceRef{{Kind: "result", ID: result.ResultID, Revision: child.Revision, Hash: final.ContentHash}, {Kind: "artifact", ID: final.ArtifactID, Hash: final.ContentHash}, {Kind: "source", ID: finalSource.ID, Revision: finalSource.Revision, Hash: finalSource.Hash}}}
	// Stored output is an observation; it does not prove any acceptance criterion.
	// In particular, do not fill Accomplished from model text or its finish reason.
	keys := make([]string, 0, len(a.reasons))
	for reason := range a.reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	for _, reason := range keys {
		r.Uncertainty = append(r.Uncertainty, delegation.Reason{Kind: reason})
	}
	r.Budget.Used, r.Budget.ReservationIDs = a.used, a.reservations
	raw, err := json.Marshal(r)
	if err != nil {
		return delegation.SourceRef{}, err
	}
	if _, err = delegation.ParseReport(raw); err != nil {
		return delegation.SourceRef{}, err
	}
	// The final-message source pins every original run source. Validate this
	// independently before using it as the sole scope-anchoring derived parent.
	roots := append(refsForDelegation(*c), pins...)
	roots = append(roots, finalSource)
	graph, err := g.sources.CanonicalSourcesTx(ctx, tx, []catalog.SourceRef{finalSource})
	if err != nil {
		return delegation.SourceRef{}, err
	}
	byID := map[string]catalog.SourceRef{}
	for _, p := range graph {
		byID[p.ID] = p
	}
	for _, p := range roots {
		if v, ok := byID[p.ID]; !ok || v.Revision != p.Revision {
			return fail()
		}
	}
	checker := delegationReportAccess{guard: g, parent: parent, child: child, contract: *c, requested: []catalog.SourceRef{finalSource}, pins: roots}
	accessCtx := catalog.WithAccessChecker(ctx, checker)
	source, err := writer.CreateDerivedTx(accessCtx, tx, checker.requested, "delegation_report", "Recorded delegated attempt", string(raw), json.RawMessage(`{"kind":"delegation_report_v1"}`))
	if err != nil {
		return delegation.SourceRef{}, err
	}
	return delegation.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}, nil
}

// Historical rows are read without taking a new run lock after current task
// locks. Mutable authority/source/effect rows still use the manager transaction.
func (g *DelegationGuards) projectionFinalTx(ctx context.Context, tx pgx.Tx, child task.Record, result task.Result) (completion.FinalRecord, catalog.SourceRef, []catalog.SourceRef, error) {
	fail := func() (completion.FinalRecord, catalog.SourceRef, []catalog.SourceRef, error) {
		return completion.FinalRecord{}, catalog.SourceRef{}, nil, delegation.ErrDenied
	}
	var f completion.FinalRecord
	var effects, limitations []byte
	err := tx.QueryRow(ctx, `SELECT f.result_id,f.run_id,f.session_key,f.task_id,f.task_revision,f.constraints_revision,f.artifact_id,f.message_id,f.content_hash,f.answer_state,f.effects_json,f.limitations_json,f.privacy_redacted FROM run_final_results f JOIN runs r USING(run_id) JOIN task_results t ON t.result_id=f.result_id WHERE f.result_id=$1 AND f.run_id=$2 AND r.current_state='completed' AND r.task_id=$3 AND r.task_revision=$4 AND r.session_key=$5 AND t.task_id=$3 AND t.task_revision=$4 AND t.run_id=$2 AND NOT t.privacy_redacted`, result.ResultID, result.RunID, child.TaskID, child.Revision, child.SessionKey).Scan(&f.ResultID, &f.RunID, &f.SessionKey, &f.TaskID, &f.TaskRevision, &f.ConstraintsRevision, &f.ArtifactID, &f.MessageID, &f.ContentHash, &f.AnswerState, &effects, &limitations, &f.PrivacyRedacted)
	if err != nil || f.PrivacyRedacted || f.TaskID != child.TaskID || f.TaskRevision != child.Revision || f.ConstraintsRevision != child.ConstraintsRevision || result.TaskID != child.TaskID || result.TaskRevision != child.Revision || json.Unmarshal(effects, &f.Effects) != nil || json.Unmarshal(limitations, &f.Limitations) != nil {
		return fail()
	}
	var artifact, message string
	err = tx.QueryRow(ctx, `SELECT a.content_text,m.content FROM artifacts a JOIN messages m ON m.message_id=$2 WHERE a.artifact_id=$1 AND a.run_id=$3 AND m.run_id=$3 AND a.session_key=$4 AND m.session_key=$4 AND a.artifact_type='assistant_final' AND m.role='assistant' AND NOT a.privacy_redacted AND NOT m.privacy_redacted`, f.ArtifactID, f.MessageID, f.RunID, child.SessionKey).Scan(&artifact, &message)
	sum := sha256.Sum256([]byte(artifact))
	if err != nil || artifact != message || hex.EncodeToString(sum[:]) != f.ContentHash {
		return fail()
	}
	var source catalog.SourceRef
	err = tx.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1 AND s.owner_id=$2 AND s.status='active' AND s.scope_type='session' AND s.scope_id=$3`, "message:"+f.MessageID, child.OwnerID, child.SessionKey).Scan(&source.ID, &source.Revision, &source.AccessRevision, &source.Hash)
	if err != nil {
		return fail()
	}
	rows, err := tx.Query(ctx, `SELECT source_id,source_revision,access_revision,content_hash FROM memory_run_sources WHERE run_id=$1 ORDER BY source_id`, f.RunID)
	if err != nil {
		return fail()
	}
	var pins []catalog.SourceRef
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
		return fail()
	}
	return f, source, pins, nil
}

func equalDelegationActions(a, b []delegation.ActionObservation) bool {
	if len(a) != len(b) {
		return false
	}
	byID := map[string]delegation.ActionObservation{}
	for _, v := range b {
		byID[v.ActionID] = v
	}
	seen := map[string]bool{}
	for _, v := range a {
		w, ok := byID[v.ActionID]
		if !ok || seen[v.ActionID] || v.EffectID != w.EffectID || v.State != w.State || !equalEvidence(v.Evidence, w.Evidence) {
			return false
		}
		seen[v.ActionID] = true
	}
	return true
}
