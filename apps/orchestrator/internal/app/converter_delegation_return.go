package app

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func (g *converterDelegationGuard) CreateReportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result) (delegation.SourceRef, error) {
	return createConverterReportTx(ctx, tx, g.DelegationGuards, parent, child, result)
}

func (g *delegationLifecycleGuard) CreateReportTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, result task.Result) (delegation.SourceRef, error) {
	return createConverterReportTx(ctx, tx, g.DelegationGuards, parent, child, result)
}

func createConverterReportTx(ctx context.Context, tx pgx.Tx, g *DelegationGuards, parent, child task.Record, result task.Result) (delegation.SourceRef, error) {
	report, err := g.CreateReportTx(ctx, tx, parent, child, result)
	if err != nil {
		return report, err
	}
	snapshot, err := generatedtools.ReadDeveloperSnapshotTx(ctx, tx, g.sources, parent.OwnerID, child.TaskID)
	if err != nil || snapshot == nil {
		return report, err
	}
	final, finalSource, pins, err := g.projectionFinalTx(ctx, tx, child, result)
	if err != nil || final.AnswerState != "final" {
		return delegation.SourceRef{}, delegation.ErrDenied
	}
	a, err := g.reportTx(ctx, tx, parent, child, result, report, nil)
	if err != nil {
		return delegation.SourceRef{}, err
	}
	// This gate is technical only. An ordinary model final remains unverified.
	for reason := range a.reasons {
		if reason != "verification_missing" {
			return delegation.SourceRef{}, delegation.ErrDenied
		}
	}
	if err = generatedtools.CheckDeveloperFinalActionsTx(ctx, tx, *snapshot, result.RunID); err != nil {
		return delegation.SourceRef{}, err
	}
	candidate, effect, checked, err := generatedtools.ReadTestedDeveloperCandidateTx(ctx, tx, *snapshot, result.RunID)
	if err != nil || candidate.Status != "tested" || candidate.Active {
		return delegation.SourceRef{}, delegation.ErrDenied
	}
	pinned := false
	for _, p := range pins {
		if p == candidate.BodySource {
			pinned = true
		}
	}
	if !pinned {
		return delegation.SourceRef{}, delegation.ErrDenied
	}
	requested := []catalog.SourceRef{candidate.BodySource, candidate.NeedSource, snapshot.Args.ContractSource}
	checker := converterCandidateProjectionAccess{guard: g, parent: parent, child: child, snapshot: *snapshot, requested: requested, pins: append(append([]catalog.SourceRef(nil), pins...), finalSource)}
	access := catalog.WithAccessChecker(owneridentity.WithActor(ctx, owneridentity.Actor{}), checker)
	rows, err := g.sources.AccessTx(access, tx, requested, "converter_candidate_return")
	if err != nil || len(rows) != 3 {
		return delegation.SourceRef{}, delegation.ErrDenied
	}
	var body catalog.Source
	for _, s := range rows {
		if s.ID == candidate.BodySource.ID {
			body = s
		}
	}
	p, err := generatedtools.Parse([]byte(body.Body))
	if err != nil || p.Digest != candidate.Digest || body.ScopeType != "task" || body.ScopeID != child.TaskID {
		return delegation.SourceRef{}, delegation.ErrDenied
	}
	projection := generatedtools.DeveloperReturn{Version: 1, SnapshotHash: generatedtools.Hash(*snapshot), OwnerID: parent.OwnerID, ParentTaskID: parent.TaskID, ChildTaskID: child.TaskID, ChildRunID: result.RunID, ResultID: result.ResultID, FinalHash: final.ContentHash, ReportSource: catalog.SourceRef(report), FinalSource: finalSource, ContractSource: snapshot.Args.ContractSource, Candidate: candidate, TestEffectID: effect, TestReport: checked}
	raw, err := json.Marshal(projection)
	if err != nil {
		return delegation.SourceRef{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO converter_developer_returns(child_task_id,owner_id,parent_task_id,result_id,child_run_id,digest,projection_hash,projection_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`, child.TaskID, parent.OwnerID, parent.TaskID, result.ResultID, result.RunID, candidate.Digest, generatedtools.Hash(projection), raw)
	return report, err
}

type converterCandidateProjectionAccess struct {
	guard           *DelegationGuards
	parent, child   task.Record
	snapshot        generatedtools.DeveloperSnapshot
	requested, pins []catalog.SourceRef
}

func (c converterCandidateProjectionAccess) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	if purpose != "converter_candidate_return" || !reflect.DeepEqual(refs, c.requested) {
		return "", delegation.ErrDenied
	}
	if err := currentDelegationTask(ctx, tx, c.parent); err != nil {
		return "", err
	}
	if err := currentDelegationTask(ctx, tx, c.child); err != nil {
		return "", err
	}
	snapshot, err := generatedtools.ReadDeveloperSnapshotTx(ctx, tx, c.guard.sources, c.parent.OwnerID, c.child.TaskID)
	if err != nil || snapshot == nil || generatedtools.Hash(*snapshot) != generatedtools.Hash(c.snapshot) {
		return "", delegation.ErrDenied
	}
	return c.parent.OwnerID, nil
}
func (c converterCandidateProjectionAccess) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) error {
	_, err := c.CheckMemoryAccessTx(ctx, tx, refs, purpose)
	return err
}
func (c converterCandidateProjectionAccess) PinnedMemorySources() []catalog.SourceRef {
	return append([]catalog.SourceRef(nil), c.pins...)
}
