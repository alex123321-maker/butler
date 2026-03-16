package doctor

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/memory/pipeline"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

type Reporter struct {
	queue    *pipeline.Queue
	postgres *postgresstore.Store
}

func NewReporter(queue *pipeline.Queue, postgres *postgresstore.Store) *Reporter {
	return &Reporter{queue: queue, postgres: postgres}
}

func (r *Reporter) Report(ctx context.Context) (map[string]any, error) {
	report := map[string]any{
		"queue":    map[string]any{"healthy": r.queue != nil},
		"pgvector": map[string]any{"healthy": false},
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
