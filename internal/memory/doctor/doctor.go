package doctor

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/memory/pipeline"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

// Reporter produces memory subsystem health reports for the doctor endpoint.
type Reporter struct {
	queue               *pipeline.Queue
	postgres            *postgresstore.Store
	pipelineEnabled     bool
	embeddingConfigured bool
}

// NewReporter creates a Reporter with the given queue and postgres store.
// Use SetPipelineEnabled and SetEmbeddingConfigured to reflect runtime state.
func NewReporter(queue *pipeline.Queue, postgres *postgresstore.Store) *Reporter {
	return &Reporter{queue: queue, postgres: postgres}
}

// SetPipelineEnabled records whether the async memory pipeline worker is active.
func (r *Reporter) SetPipelineEnabled(enabled bool) {
	r.pipelineEnabled = enabled
}

// SetEmbeddingConfigured records whether an embedding provider is configured.
func (r *Reporter) SetEmbeddingConfigured(configured bool) {
	r.embeddingConfigured = configured
}

// Report returns a structured health report for the memory subsystem.
func (r *Reporter) Report(ctx context.Context) (map[string]any, error) {
	report := map[string]any{
		"queue":    map[string]any{"healthy": r.queue != nil},
		"pgvector": map[string]any{"healthy": false},
		"pipeline_worker": map[string]any{
			"enabled": r.pipelineEnabled,
			"healthy": r.pipelineEnabled,
		},
		"embedding_provider": map[string]any{
			"configured": r.embeddingConfigured,
			"healthy":    r.embeddingConfigured,
		},
	}
	if r.queue != nil {
		depth, err := r.queue.Depth(ctx)
		if err != nil {
			return nil, fmt.Errorf("memory queue depth: %w", err)
		}
		report["queue"] = map[string]any{"healthy": true, "depth": depth}
	}
	if r.postgres != nil {
		var enabled bool
		if err := r.postgres.Pool().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&enabled); err != nil {
			return nil, fmt.Errorf("pgvector readiness: %w", err)
		}
		report["pgvector"] = map[string]any{"healthy": enabled, "enabled": enabled}
	}
	return report, nil
}
