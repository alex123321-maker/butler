package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerOverviewBackgroundQueueRetainsObsoleteAndExpiredJobsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "queue"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('queue','owner','web')`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	root, err := store.Create(ctx, CreateRequest{SessionKey: "queue", OwnerID: "owner", Goal: "root", CompletionCriteria: []string{"evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.Create(ctx, CreateRequest{SessionKey: "queue", OwnerID: "owner", ParentTaskID: root.TaskID, Goal: "child", CompletionCriteria: []string{"evidence"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE tasks SET revision=2 WHERE task_id=$1`, child.TaskID); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"current", "expired", "obsolete", "foreign"} {
		owner := "owner"
		revision := 2
		if id == "foreign" {
			owner = "other-owner"
		}
		if id == "obsolete" {
			revision = 1
		}
		if _, err := pool.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,task_id,task_revision,constraints_revision,kind,purpose,sources,source_fingerprint,allowance,deadline,status,attempts,error_code)
 VALUES($1,$2,$3,$4,1,'extract','PRIVATE_QUEUE_MARKER','[{"private":"PRIVATE_QUEUE_MARKER"}]','fixture','{"provider":"local","model":"pinned-extractor","private":"PRIVATE_QUEUE_MARKER"}',clock_timestamp()+CASE WHEN $5=1 THEN INTERVAL '-1 minute' ELSE INTERVAL '1 hour' END,'pending',1,'PRIVATE_QUEUE_MARKER')`, id, owner, child.TaskID, revision, i); err != nil {
			t.Fatal(err)
		}
	}
	v, err := store.OwnerOverview(ctx, root.TaskID)
	if err != nil || len(v.Auxiliary) != 3 || v.AuxiliaryTruncated {
		t.Fatal("missing queue or foreign job disclosure", err)
	}
	seen := map[string]AuxiliaryOverview{}
	for _, job := range v.Auxiliary {
		seen[job.ID] = job
		if job.TaskID != child.TaskID || job.RequestedModel != "pinned-extractor" || job.Status != "pending" || job.Attempts != 1 {
			t.Fatal("durable state lost")
		}
	}
	if !seen["current"].CurrentTaskVersion || seen["current"].DeadlineElapsed || !seen["expired"].DeadlineElapsed || seen["obsolete"].CurrentTaskVersion {
		t.Fatal("old revision or expired deadline presented as current")
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "PRIVATE_QUEUE_MARKER") {
		t.Fatal("raw purpose, source, error or allowance content disclosed")
	}
	// Queue volume is bounded, with explicit truncation; no hidden zero total.
	for i := 0; i < 51; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,task_id,task_revision,constraints_revision,kind,purpose,sources,source_fingerprint,allowance,deadline) VALUES($1,'owner',$2,1,1,'summary','summary','[]','fixture','{}',clock_timestamp()+INTERVAL '1 hour')`, fmt.Sprintf("bounded-%02d", i), root.TaskID); err != nil {
			t.Fatal(err)
		}
	}
	v, err = store.OwnerOverview(ctx, root.TaskID)
	if err != nil || len(v.Auxiliary) != 50 || !v.AuxiliaryTruncated {
		t.Fatal("queue limit not explicit", err)
	}
}
