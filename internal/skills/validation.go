package skills

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
)

func Validate(ctx context.Context, p Package, owner string, verifier EvidenceVerifier, reader ContractReader) (Validation, error) {
	report := Validation{Validator: "butler.procedure-checks.v1", Checks: []string{}, Sources: []SourceProof{}}
	if validate(p.Manifest) != nil || owner == "" || verifier == nil || reader == nil {
		return report, ErrUnavailable
	}
	proofs, err := verifier.VerifySkillSources(ctx, owner, p.Manifest.Sources)
	if err != nil || len(proofs) != len(p.Manifest.Sources) {
		return report, ErrEvidence
	}
	covered := map[string]bool{}
	for i, proof := range proofs {
		if proof.EvidenceRef != p.Manifest.Sources[i] || proof.OwnerID != owner || proof.Verifier != "owner" || proof.Verdict != "verified" || len(proof.EvidenceRefs) == 0 || len(proof.ConfirmedActions) == 0 || len(proof.ContentParents) == 0 {
			return report, ErrEvidence
		}
		for _, d := range proof.ConfirmedContracts {
			covered[d.Tool+"|"+d.ContractHash+"|"+d.ContractRevision] = true
		}
	}
	contracts := map[string]*toolbrokerv1.ToolContract{}
	for _, d := range p.Manifest.Dependencies {
		c, e := reader.GetSkillContract(ctx, d.Tool)
		hash, he := actionauth.ContractHash(c)
		if e != nil || he != nil || hash != d.ContractHash || c.GetContractRevision() != d.ContractRevision || !covered[d.Tool+"|"+d.ContractHash+"|"+d.ContractRevision] {
			return report, ErrConflict
		}
		contracts[d.Tool] = c
	}
	for _, step := range p.Manifest.Steps {
		effect := actionauth.CatalogEffect(contracts[step.Tool])
		// Procedure wording cannot assert replay safety. Only declared fixed read
		// contracts may recommend re-observation; mutations stop/reconcile.
		if step.Retry == "read_again" && effect != actionauth.EffectRead || effect != actionauth.EffectRead && !step.ApprovalCheckpoint {
			return report, ErrManifest
		}
	}
	roots := []catalog.SourceRef{}
	for _, proof := range proofs {
		roots = append(roots, proof.ContentParents...)
	}
	if r, ok := ctx.Value(proposalSourceKey{}).(catalog.SourceRef); ok {
		roots = append(roots, r)
	}
	if lineage, ok := verifier.(LessonVerifier); ok {
		report.LessonParents, err = lineage.VerifyLessonLineage(ctx, owner, LessonRequest{Roots: roots, Lessons: p.Manifest.LessonSources, Proofs: proofs, Dependencies: p.Manifest.Dependencies})
		if err != nil {
			return report, ErrEvidence
		}
		if len(report.LessonParents) != len(p.Manifest.LessonSources) || len(report.LessonParents) == 1 {
			return report, ErrEvidence
		}
		claimed := map[catalog.SourceRef]bool{}
		for _, r := range p.Manifest.LessonSources {
			claimed[r] = true
		}
		for _, r := range report.LessonParents {
			if !claimed[r] {
				return report, ErrEvidence
			}
			delete(claimed, r)
		}
		if len(claimed) != 0 {
			return report, ErrEvidence
		}
	} else if len(p.Manifest.LessonSources) > 0 {
		return report, ErrEvidence
	}
	// These independent fixed checks, not caller-reported pass flags, form the
	// v1 validation class. Full procedure trials supply the verified task proof.
	report.Checks = []string{"owner_verified_task_and_recipient_evidence", "exact_tool_contracts", "stop_on_failure_and_unknown_effect", "reject_changed_interface", "no_instruction_authority_or_new_capability"}
	if len(report.LessonParents) > 0 {
		report.Checks = append(report.Checks, "repeated_independent_current_experience")
	}
	report.Sources = proofs
	report.Passed = true
	return report, nil
}
