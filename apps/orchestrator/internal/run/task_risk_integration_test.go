package run_test

import (
	"context"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestTaskListDoesNotInferActionRiskFromRunStateIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('risk-owner','risk-owner','web')`); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(pool)
	for _, state := range []string{"failed", "tool_running", "completed"} {
		item, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "risk-owner", OwnerID: "risk-owner", Goal: state, CompletionCriteria: []string{"verified evidence"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,'risk-owner',$1,$2,$2,'mode_2','fixture',clock_timestamp(),clock_timestamp(),$3,1)`, state, state, item.TaskID); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE tasks SET last_run_id=$2 WHERE task_id=$1`, item.TaskID, state); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := run.NewPostgresRepository(pool).ListTasks(ctx, run.TaskListParams{})
	if err != nil || len(rows) != 3 {
		t.Fatal("task list unavailable", err)
	}
	for _, row := range rows {
		if row.RiskLevel != "unknown" || row.ResultVerified || row.TaskStatus != "active" {
			t.Fatalf("run state promoted to task success or risk: %+v", row)
		}
	}
}
