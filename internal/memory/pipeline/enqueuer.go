package pipeline

import (
	"context"
	"time"

	"github.com/butler/butler/internal/metrics"
)

// Enqueuer implements the orchestrator's PipelineEnqueuer interface using
// the pipeline Queue.
type Enqueuer struct {
	queue   *Queue
	metrics *metrics.Registry
}

// NewEnqueuer creates a new pipeline enqueuer.
func NewEnqueuer(queue *Queue, registry *metrics.Registry) *Enqueuer {
	return &Enqueuer{queue: queue, metrics: registry}
}

// EnqueuePostRun enqueues a post-run memory pipeline job.
func (e *Enqueuer) EnqueuePostRun(ctx context.Context, runID, sessionKey string) error {
	err := e.queue.Enqueue(ctx, Job{
		JobType:    JobTypePostRun,
		RunID:      runID,
		SessionKey: sessionKey,
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if e.metrics != nil {
		status := "success"
		if err != nil {
			status = "error"
		}
		_ = e.metrics.IncrCounter(metrics.MetricMemoryJobsTotal, map[string]string{"job_type": JobTypePostRun, "service": "memory-pipeline", "status": status})
		if depth, depthErr := e.queue.Depth(ctx); depthErr == nil {
			_ = e.metrics.SetGauge(metrics.MetricMemoryQueueDepth, float64(depth), map[string]string{"queue": QueueKey, "service": "memory-pipeline"})
		}
	}
	return err
}
