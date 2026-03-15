package pipeline

import (
	"context"
	"time"
)

// Enqueuer implements the orchestrator's PipelineEnqueuer interface using
// the pipeline Queue.
type Enqueuer struct {
	queue *Queue
}

// NewEnqueuer creates a new pipeline enqueuer.
func NewEnqueuer(queue *Queue) *Enqueuer {
	return &Enqueuer{queue: queue}
}

// EnqueuePostRun enqueues a post-run memory pipeline job.
func (e *Enqueuer) EnqueuePostRun(ctx context.Context, runID, sessionKey string) error {
	return e.queue.Enqueue(ctx, Job{
		JobType:    JobTypePostRun,
		RunID:      runID,
		SessionKey: sessionKey,
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
