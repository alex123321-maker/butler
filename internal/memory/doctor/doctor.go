// Package doctor reports observations without inference or writes.
package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/diagnostics"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/owneridentity"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

type queueReader interface {
	Depth(context.Context) (int64, error)
}
type storageReader interface {
	Stores(context.Context) map[string]diagnostics.Component
	Jobs(context.Context, string) (JobObservation, error)
	Indexes(context.Context, string) ([]IndexObservation, bool, error)
}
type Options struct {
	PipelineConfigured, PipelineEffective bool
	SemanticConfigured, SemanticEffective bool
	EmbeddingConfigured                   bool
	Catalog                               *catalog.Store
	Metrics                               *metrics.Registry
	Configuration                         any // Content-free configuration identity, never credentials.
	Features                              []FeatureConfiguration
}
type Reporter struct {
	queue   queueReader
	storage storageReader
	options Options
}

func NewReporter(queue *pipeline.Queue, postgres *postgresstore.Store) *Reporter {
	r := &Reporter{}
	if queue != nil {
		r.queue = queue
	}
	if postgres != nil {
		r.storage = &postgresReader{pool: postgres.Pool()}
	}
	return r
}
func (r *Reporter) Configure(o Options) { r.options = o }
func (r *Reporter) SetPipelineEnabled(v bool) {
	r.options.PipelineConfigured = v
	r.options.PipelineEffective = v
}
func (r *Reporter) SetEmbeddingConfigured(v bool) { r.options.EmbeddingConfigured = v }

func feature(configured, effective bool, now time.Time, reason string) diagnostics.Component {
	c := diagnostics.Component{Configured: configured, Effective: effective, Status: "unknown", Reason: "not_observed", CheckedAt: now}
	if !configured {
		c.Status = "disabled"
		c.Reason = "not_configured"
	} else if !effective {
		c.Status = "degraded"
		c.Reason = reason
	}
	return c
}

// Per-probe deadlines preserve independent checks. Failed reads have no depth
// or count fields; an absent measurement never manufactures a zero.
func (r *Reporter) Report(ctx context.Context) (map[string]any, error) {
	ctx, stop := context.WithTimeout(ctx, 8*time.Second)
	defer stop()
	now := time.Now().UTC()
	components := map[string]diagnostics.Component{}
	if r.storage != nil {
		components = r.storage.Stores(ctx)
	}
	for _, name := range []string{"transcript", "working", "profile", "episodic", "chunks"} {
		if _, ok := components[name]; !ok {
			components[name] = diagnostics.Component{Required: true, Status: "unknown", Reason: "storage_unavailable", CheckedAt: now}
		}
	}
	components["legacy_pipeline"] = feature(r.options.PipelineConfigured, r.options.PipelineEffective, now, "durable_admission_unavailable")
	if !r.options.PipelineEffective {
		c := components["legacy_pipeline"]
		c.Status = "disabled"
		components["legacy_pipeline"] = c
	}
	components["legacy_embedding"] = feature(r.options.EmbeddingConfigured, r.options.EmbeddingConfigured, now, "")
	components["semantic_retrieval"] = feature(r.options.SemanticConfigured, r.options.SemanticEffective, now, "composition_unavailable")
	c := diagnostics.Component{Configured: r.options.Catalog != nil, Effective: r.options.Catalog != nil, Required: true, Status: "unknown", Reason: "not_observed", CheckedAt: now}
	if r.options.Catalog != nil && ctx.Err() == nil {
		probe, cancel := context.WithTimeout(ctx, time.Second)
		err := r.options.Catalog.CheckRestore(probe)
		cancel()
		c.Status = "healthy"
		c.Reason = "restore_manifest_checked"
		if err != nil {
			c.Status = "failed"
			_, c.Reason = diagnostics.ErrorOutcome(err)
		}
	}
	components["source_authority"] = c
	queue := map[string]any{"status": "unknown", "reason": "not_observed", "checked_at": now}
	if r.queue != nil && ctx.Err() == nil {
		probe, cancel := context.WithTimeout(ctx, time.Second)
		depth, err := r.queue.Depth(probe)
		cancel()
		if err == nil {
			queue = map[string]any{"status": "healthy", "depth": depth, "checked_at": time.Now().UTC()}
		} else {
			queue["status"] = "failed"
			queue["reason"] = "backend"
		}
	}
	jobs := map[string]any{"status": "unknown", "reason": "not_observed", "checked_at": now}
	indexes := map[string]any{"status": "unknown", "reason": "not_observed", "checked_at": now}
	actor, authorized := owneridentity.FromContext(ctx)
	if !authorized {
		jobs["reason"] = "owner_required"
		indexes["reason"] = "owner_required"
	} else if r.storage != nil && ctx.Err() == nil {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		observed, err := r.storage.Jobs(probe, actor.OwnerID)
		cancel()
		if err == nil {
			jobs = map[string]any{"status": "healthy", "observation": observed, "checked_at": time.Now().UTC()}
		} else {
			jobs["status"] = "failed"
			jobs["reason"] = "backend"
		}
		probe, cancel = context.WithTimeout(ctx, 2*time.Second)
		generations, truncated, err := r.storage.Indexes(probe, actor.OwnerID)
		cancel()
		if err == nil {
			permitted := []IndexObservation{}
			observationStatus, observationReason := "healthy", "scope_checked"
			if r.options.Catalog == nil {
				observationStatus, observationReason = "unknown", "scope_verification_unavailable"
			}
			for _, g := range generations {
				if r.options.Catalog != nil {
					if e := r.options.Catalog.CheckOwnerScope(ctx, g.ScopeType, g.ScopeID); e == nil {
						permitted = append(permitted, g)
					} else {
						observationStatus, observationReason = "unknown", "scope_unverified"
					}
				}
			}
			indexes = map[string]any{"status": observationStatus, "reason": observationReason, "generations": permitted, "truncated": truncated, "checked_at": time.Now().UTC()}
		} else {
			indexes["status"] = "failed"
			indexes["reason"] = "backend"
		}
	}
	featureState := r.featureReport(ctx, now)
	raw, _ := json.Marshal(map[string]any{"runtime": r.options.Configuration, "pipeline_requested": r.options.PipelineConfigured, "pipeline_connected": r.options.PipelineEffective, "semantic_requested": r.options.SemanticConfigured, "semantic_connected": r.options.SemanticEffective, "legacy_embedding": r.options.EmbeddingConfigured, "features": featureConfigurations(r.options.Features)})
	digest := sha256.Sum256(raw)
	quality := map[string]any{"status": "unverified", "reason": "no_applicable_answer_quality_report", "configuration": hex.EncodeToString(digest[:]), "checked_at": now}
	checks := make([]diagnostics.Component, 0, len(components)+2)
	for _, v := range components {
		checks = append(checks, v)
	}
	for _, v := range []map[string]any{jobs, indexes} {
		checks = append(checks, diagnostics.Component{Status: v["status"].(string), CheckedAt: now})
	}
	status := diagnostics.Aggregate(checks, time.Now().UTC(), 5*time.Minute)
	if r.options.Metrics != nil {
		_ = r.options.Metrics.IncrCounter(metrics.MetricDoctorChecksTotal, map[string]string{"component": "memory", "service": "orchestrator", "status": status})
	}
	return map[string]any{"version": diagnostics.Version, "status": status, "checked_at": now, "components": components, "legacy_queue": queue, "jobs": jobs, "indexes": indexes, "answer_quality": quality, "effective_features": featureState}, nil
}
