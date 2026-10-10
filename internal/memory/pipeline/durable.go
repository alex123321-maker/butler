package pipeline

import (
	"context"
	"time"
)

// ProcessDurablePostRun leaves retry and acknowledgement to the PostgreSQL
// auxiliary queue. It does not pass result durability through the Redis queue.
func (w *Worker) ProcessDurablePostRun(ctx context.Context, runID, sessionKey string) error {
	return w.processPostRun(ctx, w.log, &Job{JobType: JobTypePostRun, RunID: runID, SessionKey: sessionKey, EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano)})
}
