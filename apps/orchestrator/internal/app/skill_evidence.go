package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/skills"
)

// This adapter reads each owned repository through its typed API. Broker skills
// do not issue SQL against task/result/transcript tables or accept model verdicts.
type skillEvidence struct {
	tasks      *task.Store
	journal    *actionauth.PostgresStore
	transcript *transcript.Store
	budgets    *budget.Ledger
	results    *completion.Coordinator
	sources    *catalog.Store
	lessons    *experience.Store
}

func (v *skillEvidence) VerifyLessonLineage(ctx context.Context, owner string, q skills.LessonRequest) ([]catalog.SourceRef, error) {
	if v == nil || v.sources == nil || owner == "" || len(q.Roots) > 64 || len(q.Lessons) > 8 || len(q.Proofs) > 8 {
		return nil, skills.ErrEvidence
	}
	// The private service authenticates Broker. Neither a manifest nor its
	// supplied proof object is accepted as independent task evidence.
	refs := []skills.EvidenceRef{}
	for _, p := range q.Proofs {
		refs = append(refs, p.EvidenceRef)
	}
	proofs, err := v.VerifySkillSources(ctx, owner, refs)
	if err != nil {
		return nil, err
	}
	roots := append([]catalog.SourceRef{}, q.Roots...)
	for _, p := range proofs {
		roots = append(roots, p.ContentParents...)
	}
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: "private-skill-lineage"})
	reader := &experience.Store{Sources: v.sources}
	required, err := reader.Ancestors(ctx, roots)
	if err != nil {
		return nil, err
	}
	if len(required) == 0 && len(q.Lessons) == 0 {
		return []catalog.SourceRef{}, nil
	}
	if v.lessons == nil {
		return nil, skills.ErrEvidence
	}
	claimed := map[string]catalog.SourceRef{}
	for _, r := range q.Lessons {
		claimed[r.ID] = r
	}
	for _, r := range required {
		if claimed[r.ID] != r {
			return nil, skills.ErrEvidence
		}
	}
	return v.lessons.VerifyPromotion(ctx, owner, q.Lessons, proofs, q.Dependencies)
}

func skillCatalogIfEnabled(client *skills.Client) interface {
	Index(context.Context, string) ([]skills.Descriptor, error)
	Pinned(context.Context, string) ([]skills.Package, error)
} {
	if client == nil {
		return nil
	}
	return client
}

func (v *skillEvidence) VerifySkillSources(ctx context.Context, owner string, refs []skills.EvidenceRef) ([]skills.SourceProof, error) {
	if v == nil || v.tasks == nil || v.journal == nil || v.transcript == nil || owner == "" || len(refs) == 0 || len(refs) > 8 {
		return nil, skills.ErrEvidence
	}
	proofs := []skills.SourceProof{}
	for _, ref := range refs {
		t, err := v.tasks.Get(ctx, ref.TaskID)
		if err != nil || t.OwnerID != owner {
			return nil, skills.ErrEvidence
		}
		results, err := v.tasks.Results(ctx, ref.TaskID)
		if err != nil {
			return nil, skills.ErrEvidence
		}
		var result task.Result
		for _, candidate := range results {
			if candidate.ResultID == ref.ResultID {
				result = candidate
			}
		}
		if result.TaskRevision != ref.TaskRevision || result.Verifier != "owner" || result.Verdict != "verified" || result.RunID == "" || len(result.EvidenceRefs) == 0 || len(result.VerifiedCriteria) == 0 {
			return nil, skills.ErrEvidence
		}
		effects, err := v.journal.GetRunEvidence(ctx, result.RunID)
		if err != nil {
			return nil, skills.ErrEvidence
		}
		p := skills.SourceProof{EvidenceRef: ref, OwnerID: owner, Verifier: result.Verifier, Verdict: result.Verdict, ConfirmedTools: []string{}, ConfirmedActions: []string{}}
		// Owner evidence labels can contain private prose. Validation history
		// stores content-free identities; the original result/source graph owns
		// access and deletion of the actual text.
		for _, ref := range result.EvidenceRefs {
			sum := sha256.Sum256([]byte(ref))
			p.EvidenceRefs = append(p.EvidenceRefs, "owner-evidence-sha256:"+hex.EncodeToString(sum[:]))
		}
		if v.results == nil || v.sources == nil {
			return nil, skills.ErrEvidence
		}
		// Owner verification is a separate immutable task result, not an UPDATE
		// of the model final. Bind it to the same actual final run/task revision.
		final, err := v.results.GetByRun(ctx, result.RunID)
		if err != nil || final.RunID != result.RunID || final.TaskID != ref.TaskID || final.TaskRevision != ref.TaskRevision {
			return nil, skills.ErrEvidence
		}
		p.ContentParents, err = v.sources.VerifiedResultParents(ctx, owner, final.MessageID, final.RunID)
		if err != nil || len(p.ContentParents) == 0 {
			return nil, skills.ErrEvidence
		}
		seen := map[string]bool{}
		for _, effect := range effects {
			if effect.EffectState != actionauth.StateConfirmed || len(effect.EvidenceRefs) == 0 {
				continue
			}
			e, err := v.journal.GetEffect(ctx, effect.ActionID)
			if err != nil || e.Binding.OwnerID != owner || e.Binding.TaskID != ref.TaskID || e.Binding.TaskRevision != ref.TaskRevision {
				return nil, skills.ErrEvidence
			}
			p.ConfirmedActions = append(p.ConfirmedActions, e.ActionID)
			p.ConfirmedContracts = append(p.ConfirmedContracts, skills.Dependency{Tool: e.Binding.ToolName, ContractHash: e.Binding.ContractHash, ContractRevision: e.Binding.ContractRevision})
			if !seen[e.Binding.ToolName] {
				p.ConfirmedTools = append(p.ConfirmedTools, e.Binding.ToolName)
				seen[e.Binding.ToolName] = true
			}
		}
		if len(p.ConfirmedActions) == 0 {
			return nil, skills.ErrEvidence
		}
		history, err := v.transcript.GetTranscript(ctx, t.SessionKey)
		if err != nil {
			return nil, skills.ErrEvidence
		}
		for _, call := range history.ToolCalls {
			if call.RunID == result.RunID && (call.Status == "failed" || call.Status == "effect_unknown" || call.Status == "execution_stopped") {
				p.Failures++
			}
		}
		if v.budgets != nil {
			usage, e := v.budgets.ModelRunUsage(ctx, owner, result.RunID)
			if e != nil {
				return nil, skills.ErrEvidence
			}
			p.ModelAttempts = usage.Attempts
			p.ModelUsage = &usage
		}
		sort.Strings(p.ConfirmedTools)
		sort.Strings(p.ConfirmedActions)
		proofs = append(proofs, p)
	}
	return proofs, nil
}

func (v *skillEvidence) ReadSkillOutcomes(ctx context.Context, owner string, usages []skills.Usage) ([]skills.Outcome, error) {
	if v == nil || v.tasks == nil || v.transcript == nil || owner == "" || len(usages) > 200 {
		return nil, skills.ErrEvidence
	}
	result := []skills.Outcome{}
	for _, u := range usages {
		t, err := v.tasks.GetByRun(ctx, u.RunID)
		if err != nil || t.TaskID != u.TaskID || t.OwnerID != owner {
			return nil, skills.ErrAccess
		}
		o := skills.Outcome{Usage: u, Verdict: "unverified"}
		results, err := v.tasks.Results(ctx, u.TaskID)
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			if r.RunID == u.RunID && r.TaskRevision == u.TaskRevision && r.Verifier == "owner" && r.Verdict == "verified" {
				o.ResultID = r.ResultID
				o.Verdict = "verified"
			}
		}
		history, err := v.transcript.GetTranscript(ctx, t.SessionKey)
		if err != nil {
			return nil, err
		}
		for _, call := range history.ToolCalls {
			if call.RunID == u.RunID && (call.Status == "failed" || call.Status == "effect_unknown" || call.Status == "execution_stopped") {
				o.KnownToolFailures++
			}
		}
		if v.budgets != nil {
			usage, err := v.budgets.ModelRunUsage(ctx, owner, u.RunID)
			if err != nil {
				return nil, err
			}
			o.ModelUsage = &usage
		}
		result = append(result, o)
	}
	return result, nil
}
