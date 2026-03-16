package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

const (
	// QueueKey is the Redis list key for pending memory pipeline jobs.
	QueueKey = "butler:memory:pipeline:jobs"

	// JobTypePostRun is enqueued after a run completes successfully.
	JobTypePostRun = "post_run"
)

// Job represents a memory pipeline job enqueued after a run completes.
type Job struct {
	JobType    string `json:"job_type"`
	RunID      string `json:"run_id"`
	SessionKey string `json:"session_key"`
	EnqueuedAt string `json:"enqueued_at"`
}

// Queue provides methods to enqueue and dequeue memory pipeline jobs via Redis.
type Queue struct {
	client *redislib.Client
}

// NewQueue creates a new pipeline job queue backed by the given Redis client.
func NewQueue(client *redislib.Client) *Queue {
	return &Queue{client: client}
}

// Enqueue adds a job to the pipeline queue.
func (q *Queue) Enqueue(ctx context.Context, job Job) error {
	if job.EnqueuedAt == "" {
		job.EnqueuedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal pipeline job: %w", err)
	}
	if err := q.client.LPush(ctx, QueueKey, data).Err(); err != nil {
		return fmt.Errorf("enqueue pipeline job: %w", err)
	}
	return nil
}

// Dequeue blocks up to timeout waiting for a job from the pipeline queue.
// Returns nil, nil if the timeout expires without a job.
func (q *Queue) Dequeue(ctx context.Context, timeout time.Duration) (*Job, error) {
	result, err := q.client.BRPop(ctx, timeout, QueueKey).Result()
	if err != nil {
		if err == redislib.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("dequeue pipeline job: %w", err)
	}
	if len(result) < 2 {
		return nil, nil
	}
	var job Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("unmarshal pipeline job: %w", err)
	}
	return &job, nil
}

func (q *Queue) Depth(ctx context.Context) (int64, error) {
	depth, err := q.client.LLen(ctx, QueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("memory pipeline queue depth: %w", err)
	}
	return depth, nil
}
