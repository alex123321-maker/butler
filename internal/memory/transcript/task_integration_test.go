package transcript

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	// Invalidate privacy immediately after the message query has drained, before
	// any subsequent head query. The old bytes must retain their old revision.
	tracer := &taskTranscriptInvalidationTracer{pool: pool}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = tracer
	interleaved, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer interleaved.Close()
	stale, e := NewStore(interleaved).GetTaskTranscript(ctx, "mobile", "batch-b")
	if e != nil || tracer.err != nil {
		t.Fatal("interleaved transcript read failed", e, tracer.err)
	}
	var current int64
	if e = pool.QueryRow(ctx, `SELECT source_revision FROM transcript_heads WHERE session_key='mobile'`).Scan(&current); e != nil {
		t.Fatal(e)
	}
	if len(stale.Messages) != 1 || stale.SourceRevision != all.SourceRevision || current != stale.SourceRevision+1 {
		t.Fatalf("stale bytes acquired a newer privacy revision: snapshot=%d original=%d current=%d", stale.SourceRevision, all.SourceRevision, current)
	}
}

type taskTranscriptQueryKey struct{}
type taskTranscriptInvalidationTracer struct {
	pool *pgxpool.Pool
	once sync.Once
	err  error
}

func (*taskTranscriptInvalidationTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, taskTranscriptQueryKey{}, strings.Contains(data.SQL, "r.task_id=$2"))
}

func (t *taskTranscriptInvalidationTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if selected, _ := ctx.Value(taskTranscriptQueryKey{}).(bool); selected {
		t.once.Do(func() {
			_, t.err = t.pool.Exec(ctx, `UPDATE transcript_heads SET source_revision=source_revision+1 WHERE session_key='mobile'`)
		})
	}
}
