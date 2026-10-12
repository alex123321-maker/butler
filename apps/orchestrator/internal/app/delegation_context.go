package app

import (
	"context"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/security/content"
)

// The composition reads only the actual current run's immutable contract and
// selected Catalog roots. Locator/author/credential metadata is not model input.
type delegatedContextReader struct {
	sources *catalog.Store
	manager *task.DelegationManager
}

func (a delegatedContextReader) LoadDelegatedContext(ctx context.Context, runID string) (*flow.DelegatedContext, error) {
	if a.sources == nil {
		return nil, delegation.ErrDenied
	}
	selected, err := a.sources.ReadDelegatedRun(ctx, runID)
	if err != nil || selected == nil {
		return nil, err
	}
	numeric, err := selectedNumericDelegationSource(selected)
	if err != nil {
		return nil, err
	}
	if a.manager != nil {
		// Admission maintenance cannot impersonate the child. This transition
		// repeats current contract/source checks under the actual child token,
		// before any prepared DATA can be released to its model.
		if _, err = a.manager.MarkExecuting(ctx, selected.Contract.TaskID); err != nil {
			return nil, err
		}
	}
	d := &flow.DelegatedContext{RunID: runID, SessionKey: selected.SessionKey, TaskID: selected.Contract.TaskID, ContractSource: selected.Contract.ContractSource, NumericSource: numeric}
	for _, src := range selected.Sources {
		parents := []string{}
		for _, dep := range selected.Dependencies[src.ID] {
			if dep.ID != src.ID {
				parents = append(parents, dep.ID)
			}
		}
		kind := content.Document
		if src.Kind == "model_candidate" || src.Origin.Kind == content.SubagentOutput {
			kind = content.SubagentOutput
		}
		d.SourceRefs = append(d.SourceRefs, src.ID)
		d.Items = append(d.Items, content.Data(map[string]any{"source_id": src.ID, "revision": src.Revision, "access_revision": src.AccessRevision, "content_hash": src.Hash, "title": src.Title, "body": src.Body}, content.Origin{Kind: kind, Sensitivity: content.Private, SourceRefs: []string{src.ID}, ParentRefs: parents}))
	}
	return d, nil
}

// Catalog has already proved the current immutable contract and selected roots
// under this child's actual lease. A known marker selects fixed output formatting;
// arbitrary contract/source prose cannot supply kernel instructions.
func selectedNumericDelegationSource(selected *catalog.DelegatedRunSources) (*delegation.SourceRef, error) {
	var contract *catalog.Source
	for i := range selected.Sources {
		if sourceReference(selected.Sources[i]) == selected.Contract.ContractSource {
			contract = &selected.Sources[i]
		}
	}
	if contract == nil {
		return nil, delegation.ErrDenied
	}
	ref, applicable, err := delegation.NumericResearchSource([]byte(contract.Body))
	if err != nil || !applicable {
		return nil, err
	}
	for _, root := range selected.Contract.Sources {
		if root == ref {
			for _, src := range selected.Sources {
				if sourceReference(src) == ref {
					return &ref, nil
				}
			}
		}
	}
	return nil, delegation.ErrDenied
}
