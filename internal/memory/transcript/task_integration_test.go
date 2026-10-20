package transcript

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestTaskTranscriptUsesOnlyOwnRunsAndKeepsSessionRevisionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	_, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('mobile','owner','web'),('empty','owner','web');
 INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('batch-a','mobile','owner','first batch'),('batch-b','mobile','owner','second batch');
 INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,task_id,started_at,updated_at) VALUES('run-a','mobile','a','completed','mode_2','completed','test','batch-a',clock_timestamp(),clock_timestamp()),('run-b','mobile','b','completed','mode_2','completed','test','batch-b',clock_timestamp(),clock_timestamp());
 INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('a','mobile','run-a','user','first batch history'),('b','mobile','run-b','user','second batch history');`)
	if e != nil {
		t.Fatal(e)
	}
	s := NewStore(pool)
	all, e := s.GetTranscript(ctx, "mobile")
	if e != nil {
		t.Fatal(e)
	}
	selected, e := s.GetTaskTranscript(ctx, "mobile", "batch-b")
	if e != nil {
		t.Fatal(e)
	}
	if len(selected.Messages) != 1 || selected.Messages[0].Content != "second batch history" || selected.SourceRevision != all.SourceRevision {
		t.Fatal("task selection or privacy revision lost")
	}
	if len(all.Messages) != 2 {
		t.Fatal("task projection changed audit history")
	}
	empty, e := s.GetTaskTranscript(ctx, "empty", "new-batch")
	if e != nil || len(empty.Messages) != 0 || empty.SourceRevision != 0 {
		t.Fatal("new empty session cannot start", e)
	}
}
