package skills

import "context"

// Quality compares independently verified task outcomes and measured attempts /
// tokens with source trials. It never calls a model or counts creation as success.
func (s *Store) Quality(ctx context.Context, hash string) (QualityReport, error) {
	r := QualityReport{Digest: hash, Baseline: []SourceProof{}, Outcomes: []Outcome{}}
	if _, err := s.actor(ctx); err != nil {
		return r, err
	}
	p, err := s.Get(ctx, hash)
	if err != nil {
		return r, err
	}
	r.Baseline, err = s.verifier.VerifySkillSources(ctx, s.owner, p.Package.Manifest.Sources)
	if err != nil {
		return r, err
	}
	usages, err := s.Usages(ctx, hash)
	if err != nil {
		return r, err
	}
	if len(usages) == 0 {
		return r, nil
	}
	reader, ok := s.verifier.(OutcomeReader)
	if !ok {
		return r, ErrUnavailable
	}
	r.Outcomes, err = reader.ReadSkillOutcomes(ctx, s.owner, usages)
	if err != nil || len(r.Outcomes) != len(usages) {
		return r, ErrUnavailable
	}
	for i, o := range r.Outcomes {
		if o.Usage != usages[i] {
			return r, ErrEvidence
		}
		if o.Verdict == "verified" {
			r.VerifiedSuccesses++
		} else {
			r.UnverifiedAttempts++
		}
		r.KnownToolFailures += o.KnownToolFailures
	}
	return r, nil
}
