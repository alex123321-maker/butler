package orchestrator

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

// This trusted composition derives immediate children from the actual parent
// run. It cannot accept model-selected tasks, bodies or completion claims.
type FailedDelegationAttemptReader interface {
	LoadFailedDelegationAttempts(context.Context, string) ([]delegation.AttemptReport, error)
}

const failedAttemptMetadata = "butler_failed_delegation_attempt"

func isFailedAttemptItem(item transport.InputItem) bool {
	marked, _ := item.Metadata[failedAttemptMetadata].(bool)
	return marked
}

func failedAttemptItem(value any, refs []string) transport.InputItem {
	item := content.Data(value, content.Origin{Kind: content.SubagentOutput, Sensitivity: content.Private, SourceRefs: refs})
	item.Metadata[failedAttemptMetadata] = true
	return item
}

// Always replace saved observations. A journal may have been reconciled, or a
// source may have been corrected/deleted, while the parent worker was absent.
func (s *Service) attachFailedDelegationAttempts(ctx context.Context, p *preparedRun) error {
	previous := len(p.AttemptSourceRefs) != 0
	items := make([]transport.InputItem, 0, len(p.InputItems))
	for _, item := range p.InputItems {
		if isFailedAttemptItem(item) {
			previous = true
			continue
		}
		items = append(items, item)
	}
	p.InputItems, p.AttemptSourceRefs = items, nil
	if s.config.FailedDelegationAttempts == nil {
		if previous {
			return delegation.ErrDenied
		}
		return nil
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != p.CurrentRunID {
		return delegation.ErrDenied
	}
	reports, err := s.config.FailedDelegationAttempts.LoadFailedDelegationAttempts(ctx, token.RunID)
	if err != nil {
		return err
	}
	if len(reports) > 8 {
		return delegation.ErrDenied
	}
	seenTasks, seenSources, totalBytes := map[string]bool{}, map[string]bool{}, 0
	for _, r := range reports {
		if p.Task == nil || p.Task.Status != "active" || p.Task.SessionKey != token.SessionKey || r.ParentTaskID != p.Task.TaskID || r.TaskID == "" || r.TaskID == r.ParentTaskID || seenTasks[r.TaskID] || r.TaskStatus != "blocked" || r.TaskRevision < 1 || r.ConstraintsRevision < 1 || r.ContractRevision < 1 || r.ObservedAt.IsZero() || len(r.Attempts) == 0 || len(r.Attempts) > 16 || len(r.Sources) == 0 || len(r.Sources) > 65 || len(r.Actions) > 64 || len(r.ModelUsage) > 128 || len(r.ReservationIDs) > 128 {
			return delegation.ErrDenied
		}
		seenTasks[r.TaskID] = true
		refs := make([]string, 0, len(r.Sources))
		local := map[string]bool{}
		for _, ref := range r.Sources {
			if !delegation.ValidSource(ref) || local[ref.ID] {
				return delegation.ErrDenied
			}
			local[ref.ID] = true
			refs = append(refs, ref.ID)
			if !seenSources[ref.ID] {
				seenSources[ref.ID] = true
				p.AttemptSourceRefs = append(p.AttemptSourceRefs, ref.ID)
			}
		}
		item := failedAttemptItem(r, refs)
		if !json.Valid([]byte(item.Content)) {
			return delegation.ErrDenied
		}
		totalBytes += len(item.Content)
		if totalBytes > 256<<10 {
			return delegation.ErrDenied
		}
		p.InputItems = append(p.InputItems, item)
	}
	return nil
}
