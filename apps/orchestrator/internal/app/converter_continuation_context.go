package app

import (
	"context"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
)

func (s *ConverterContinuations) LoadConverterContinuation(ctx context.Context, event ingress.InputEvent) (flow.ConverterContinuationContext, error) {
	var out flow.ConverterContinuationContext
	var binding durableinput.ConverterContinuationBinding
	tok, owned := ownership.TokenFromContext(ctx)
	if !owned || event.Source != "converter-return" || generatedtools.Decode([]byte(event.PayloadJSON), &binding) != nil || binding.ID == "" || binding.TaskID == "" {
		return out, generatedtools.ErrAccess
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	// Unlocked routing identity only. The private verifier locks current ownership,
	// session, run and task before the immutable055/current authority/source rows.
	var c generatedtools.Continuation
	c.Intent.ID, c.Intent.ParentTaskID = binding.ID, binding.TaskID
	if tx.QueryRow(ctx, `SELECT owner_id,parent_revision,constraints_revision,input_id,state,deadline FROM converter_parent_continuations WHERE continuation_id=$1 AND parent_task_id=$2 AND state='queued'`, binding.ID, binding.TaskID).Scan(&c.Intent.OwnerID, &c.Intent.ParentRevision, &c.Intent.ConstraintsRevision, &c.InputID, &c.State, &c.Intent.Deadline) != nil {
		return out, generatedtools.ErrAccess
	}
	if err = s.inputs.VerifyConverterContinuationTx(ctx, tx, event, c); err != nil {
		return out, err
	}
	c, err = generatedtools.ReadContinuationTx(ctx, tx, binding.ID)
	if err != nil {
		return out, err
	}
	_, p, err := generatedtools.CheckContinuationTx(ctx, tx, s.sources, c)
	if err != nil {
		return out, err
	}
	if err = s.checkSourcesTx(ctx, tx, c, p); err != nil {
		return out, err
	}
	// Pin the complete original/past-parent DATA graph into this actual owned run.
	// This is ordinary Catalog access under its lease, never a synthesized actor.
	refs := append(append([]catalog.SourceRef(nil), c.Intent.OriginalSources...), c.Ready.Sources...)
	if _, err = s.sources.AccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return out, err
	}
	if err = s.inputs.VerifyConverterContinuationTx(ctx, tx, event, c); err != nil {
		return out, err
	}
	if _, _, err = generatedtools.CheckContinuationTx(ctx, tx, s.sources, c); err != nil {
		return out, err
	}
	out = flow.ConverterContinuationContext{RunID: tok.RunID, SessionKey: tok.SessionKey, TaskID: c.Intent.ParentTaskID, ID: c.Intent.ID, Purpose: c.Intent.Purpose, ChildTaskID: c.Intent.ChildTaskID}
	return out, tx.Commit(ctx)
}
