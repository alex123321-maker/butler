package app

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/internal/actionauth"
)

type journalEvidenceReader interface {
	GetRunEvidence(context.Context, string) ([]actionauth.RunEffect, error)
}
type effectEvidence struct{ journal journalEvidenceReader }

func (r effectEvidence) ReadRunEffects(ctx context.Context, runID string) ([]completion.Effect, error) {
	records, err := r.journal.GetRunEvidence(ctx, runID)
	if err != nil {
		return nil, err
	}
	result := make([]completion.Effect, 0, len(records))
	for _, record := range records {
		result = append(result, completion.Effect{ActionID: record.ActionID, EffectState: string(record.EffectState), EvidenceRefs: append([]string(nil), record.EvidenceRefs...)})
	}
	return result, nil
}
