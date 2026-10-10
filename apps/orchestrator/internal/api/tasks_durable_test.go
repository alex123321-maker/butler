package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
)

type fakeDurableTasks struct{ record task.Record }

func (f fakeDurableTasks) Get(context.Context, string) (task.Record, error) { return f.record, nil }
func (f fakeDurableTasks) RunIDs(context.Context, string) ([]string, error) {
	return []string{"run-1", "run-2"}, nil
}
func (f fakeDurableTasks) Results(context.Context, string) ([]task.Result, error) {
	return []task.Result{{ResultID: "result-1", Summary: "Draft ready", Verdict: "unverified"}}, nil
}

func TestDurableTaskDetailKeepsTaskWaitingWhenAttemptCompleted(t *testing.T) {
	server := NewTaskViewServer(nil, &fakeRunReader{record: run.Record{RunID: "run-2", CurrentState: "completed"}}, nil, nil, nil, nil, nil)
	server.SetDurableTasks(fakeDurableTasks{record: task.Record{TaskID: "task-1", SessionKey: "web:1", LastRunID: "run-2", Goal: "Prepare, approve and send", Status: "waiting_for_user", Revision: 3}})
	recorder := httptest.NewRecorder()
	server.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/tasks/task-1", nil))
	if recorder.Code != 200 {
		t.Fatal(recorder.Body.String())
	}
	var body struct {
		Task struct {
			TaskID   string `json:"task_id"`
			Status   string `json:"status"`
			RunState string `json:"run_state"`
		}
		Result struct {
			Verified bool `json:"verified"`
		}
		Runs []string `json:"run_ids"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Task.TaskID != "task-1" || body.Task.Status != "waiting_for_user" || body.Task.RunState != "completed" || body.Result.Verified || len(body.Runs) != 2 {
		t.Fatalf("task/attempt confused: %s", recorder.Body.String())
	}
}

func TestTaskCompletionEndpointRequiresExplicitOwnerVerdict(t *testing.T) {
	// The control service must reject missing owner confirmation before writing.
	server := NewTaskControlServer(nil)
	recorder := httptest.NewRecorder()
	server.HandleItem("/api/v3/tasks/").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v3/tasks/task-1/results", strings.NewReader(`{"task_revision":1,"summary":"model says finished","owner_confirmed":false}`)))
	if recorder.Code != 400 {
		t.Fatalf("model final accepted as owner verdict: %d", recorder.Code)
	}
}
