package app

import (
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
)

// An explicit numeric research contract tightens the existing owner acceptance
// path. All bodies here were read by reportTx under current Catalog/task proof
// on the manager transaction; finalText passed its immutable artifact/hash check.
// The checker grants neither verification authority nor a successful task state.
func checkNumericResearchAcceptance(a delegationReportEvidence, decision task.DelegationDecision) error {
	contractSource, ok := a.sources[a.contract.ContractSource.ID]
	if !ok || sourceReference(contractSource) != a.contract.ContractSource {
		return delegation.ErrDenied
	}
	ref, applicable, err := delegation.NumericResearchSource([]byte(contractSource.Body))
	if err != nil || !applicable {
		return err
	}
	selected, supplied := false, false
	for _, selectedRef := range a.contract.Sources {
		if selectedRef == ref {
			selected = true
		}
	}
	for _, suppliedRef := range decision.Evidence {
		if suppliedRef == ref {
			supplied = true
		}
	}
	witness, ok := a.sources[ref.ID]
	if !selected || !supplied || !ok || sourceReference(witness) != ref {
		return delegation.ErrDenied
	}
	return delegation.VerifyNumericObservation([]byte(a.finalText), ref, []byte(witness.Body))
}

func sourceReference(source catalog.Source) delegation.SourceRef {
	return delegation.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}
}
