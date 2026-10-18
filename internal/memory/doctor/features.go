package doctor

import (
	"context"
	"sort"
	"time"

	"github.com/butler/butler/internal/owneridentity"
)

// FeatureConfiguration is supplied by the composition root from the actual
// constructed services and code-owned gates. It is not owner policy, worker
// liveness, source permission, answer quality or permission to execute a job.
type FeatureConfiguration struct {
	Name         string `json:"name"`
	Requested    bool   `json:"requested"`
	Wired        bool   `json:"wired"`
	Qualified    bool   `json:"release_qualified"`
	ProfileReady bool   `json:"profile_supported"`
	Handler      string `json:"handler,omitempty"`
}

type PolicyCounts struct {
	Active    int64 `json:"active"`
	Paused    int64 `json:"paused"`
	Revoked   int64 `json:"revoked"`
	Expired   int64 `json:"expired"`
	Exhausted int64 `json:"job_allowance_exhausted"`
}

type FeaturePolicies struct {
	WorkerControl string                  `json:"worker_control"`
	Handlers      map[string]PolicyCounts `json:"handlers"`
}

type featurePolicyReader interface {
	FeaturePolicies(context.Context, string, []string) (FeaturePolicies, error)
}

func knownFeature(name string) bool {
	switch name {
	case "source_reads", "candidate_proposals", "candidate_jobs", "task_compaction", "adaptive_retrieval", "temporal_facts", "consolidation", "experience", "experience_context":
		return true
	}
	return false
}

func featureConfigurations(configured []FeatureConfiguration) []FeatureConfiguration {
	result := make([]FeatureConfiguration, 0, len(configured))
	seen := map[string]bool{}
	for _, f := range configured {
		if knownFeature(f.Name) && !seen[f.Name] {
			result = append(result, f)
			seen[f.Name] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (r *Reporter) featureReport(ctx context.Context, now time.Time) map[string]any {
	configurations := featureConfigurations(r.options.Features)
	features := make([]map[string]any, 0, len(configurations))
	handlers := []string{}
	for _, f := range configurations {
		state, reason := "disabled", "not_requested"
		if f.Requested {
			switch {
			case !f.Qualified:
				state, reason = "blocked", "release_not_qualified"
			case !f.ProfileReady:
				state, reason = "blocked", "profile_not_supported"
			case !f.Wired:
				state, reason = "blocked", "composition_unavailable"
			default:
				state, reason = "composed", "current_authority_required"
			}
		}
		features = append(features, map[string]any{"configuration": f, "state": state, "reason": reason})
		if f.Handler != "" {
			handlers = append(handlers, f.Handler)
		}
	}
	policies := map[string]any{"status": "unknown", "reason": "not_observed", "checked_at": now}
	actor, authorized := owneridentity.FromContext(ctx)
	if !authorized {
		policies["reason"] = "owner_required"
	} else if reader, ok := r.storage.(featurePolicyReader); ok && ctx.Err() == nil {
		probe, stop := context.WithTimeout(ctx, 2*time.Second)
		observation, err := reader.FeaturePolicies(probe, actor.OwnerID, handlers)
		stop()
		if err != nil {
			policies["status"], policies["reason"] = "failed", "backend"
		} else {
			policies = map[string]any{"status": "observed", "reason": "metadata_only_not_admission", "checked_at": time.Now().UTC(), "observation": observation}
		}
	}
	return map[string]any{"features": features, "owner_policies": policies, "quality": "unverified", "scope": "composition_and_durable_controls_only"}
}

func (p *postgresReader) FeaturePolicies(ctx context.Context, owner string, handlers []string) (FeaturePolicies, error) {
	out := FeaturePolicies{Handlers: map[string]PolicyCounts{}}
	if err := p.pool.QueryRow(ctx, `SELECT state FROM memory_model_control WHERE singleton`).Scan(&out.WorkerControl); err != nil {
		return out, err
	}
	// These are counts, not an admission decision: sources, task state, privacy,
	// quiet time, budgets and locks are checked separately by the owning worker.
	rows, err := p.pool.Query(ctx, `SELECT policy->>'handler',count(*) FILTER(WHERE state='active'),count(*) FILTER(WHERE state='paused'),count(*) FILTER(WHERE state='revoked'),count(*) FILTER(WHERE (policy#>>'{job,deadline}')::timestamptz<=clock_timestamp()),count(*) FILTER(WHERE jobs_created >= (policy->>'max_jobs')::int) FROM memory_model_policies WHERE owner_id=$1 AND policy->>'handler'=ANY($2::text[]) GROUP BY 1`, owner, handlers)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var handler string
		var counts PolicyCounts
		if err = rows.Scan(&handler, &counts.Active, &counts.Paused, &counts.Revoked, &counts.Expired, &counts.Exhausted); err != nil {
			return out, err
		}
		out.Handlers[handler] = counts
	}
	return out, rows.Err()
}
