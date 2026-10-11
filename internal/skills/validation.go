package skills

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
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
	// These independent fixed checks, not caller-reported pass flags, form the
	// v1 validation class. Full procedure trials supply the verified task proof.
	report.Checks = []string{"owner_verified_task_and_recipient_evidence", "exact_tool_contracts", "stop_on_failure_and_unknown_effect", "reject_changed_interface", "no_instruction_authority_or_new_capability"}
	report.Sources = proofs
	report.Passed = true
	return report, nil
}
