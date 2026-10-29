package app

import (
	"encoding/json"
	"fmt"
	"testing"
)

type laba115ProjectionTask struct {
	ID          string `json:"task_id"`
	Owner       string `json:"owner_id"`
	Session     string `json:"session_key"`
	Revision    int64  `json:"revision"`
	Constraints int64  `json:"constraints_revision"`
}

type laba115ProjectionRun struct {
	ID          string `json:"run_id"`
	Task        string `json:"task_id"`
	Session     string `json:"session_key"`
	Revision    int64  `json:"task_revision"`
	Constraints int64  `json:"constraints_revision"`
	State       string `json:"current_state"`
}

// A failed Flow may return no ExecutionResult after actually admitted calls.
// Recover facts only from the exact task, durable input and run, never from a
// latest-run guess. This does not synthesize an answer, success or authority.
func laba115ProjectionOwnedRun(db map[string]json.RawMessage, task laba115ProjectionTask) (laba115ProjectionRun, error) {
	var zero laba115ProjectionRun
	if task.ID == "" || task.Owner == "" || task.Session == "" || task.Revision < 1 || task.Constraints < 1 {
		return zero, fmt.Errorf("exact owning task required")
	}
	var tasks []laba115ProjectionTask
	var runs []laba115ProjectionRun
	var inputs []struct {
		Run     string `json:"run_id"`
		Task    string `json:"task_id"`
		Owner   string `json:"owner_id"`
		Session string `json:"session_key"`
	}
	if json.Unmarshal(db["tasks"], &tasks) != nil || json.Unmarshal(db["runs"], &runs) != nil || json.Unmarshal(db["accepted_inputs"], &inputs) != nil {
		return zero, fmt.Errorf("complete owning row population required")
	}
	current := 0
	for _, row := range tasks {
		if row.ID == task.ID && row.Owner == task.Owner && row.Session == task.Session && row.Revision >= task.Revision && row.Constraints == task.Constraints {
			current++
		}
	}
	if current != 1 {
		return zero, fmt.Errorf("unique current task identity required")
	}
	candidates := map[string]laba115ProjectionRun{}
	for _, input := range inputs {
		if input.Run == "" || input.Task != task.ID || input.Owner != task.Owner || input.Session != task.Session {
			continue
		}
		for _, run := range runs {
			if run.ID == input.Run && run.Task == task.ID && run.Session == task.Session && run.Revision == task.Revision {
				candidates[run.ID] = run
			}
		}
	}
	if len(candidates) != 1 {
		return zero, fmt.Errorf("one corroborated current input/run required")
	}
	for _, run := range candidates {
		return run, nil
	}
	return zero, fmt.Errorf("current run missing")
}

func TestLaba115ProjectionOwnedRunRejectsAmbiguityAndForeignRows(t *testing.T) {
	task := laba115ProjectionTask{ID: "current", Owner: "owner", Session: "web:current", Revision: 2, Constraints: 3}
	fixture := func() map[string]json.RawMessage {
		return map[string]json.RawMessage{
			"tasks":           json.RawMessage(`[{"task_id":"current","owner_id":"owner","session_key":"web:current","revision":2,"constraints_revision":3}]`),
			"runs":            json.RawMessage(`[{"run_id":"failed","task_id":"current","session_key":"web:current","task_revision":2,"constraints_revision":3,"current_state":"failed"},{"run_id":"other","task_id":"foreign","session_key":"web:foreign","task_revision":2,"constraints_revision":3,"current_state":"completed"}]`),
			"accepted_inputs": json.RawMessage(`[{"run_id":"failed","task_id":"current","owner_id":"owner","session_key":"web:current"}]`),
		}
	}
	got, err := laba115ProjectionOwnedRun(fixture(), task)
	if err != nil || got.ID != "failed" || got.State != "failed" {
		t.Fatal("failed run facts disappeared or latest foreign run used", err)
	}
	terminal := fixture()
	terminal["tasks"] = json.RawMessage(`[{"task_id":"current","owner_id":"owner","session_key":"web:current","revision":3,"constraints_revision":3}]`)
	if _, err = laba115ProjectionOwnedRun(terminal, task); err != nil {
		t.Fatal("terminal task revision erased captured run", err)
	}
	for _, row := range []string{
		`[{"run_id":"other","task_id":"current","owner_id":"owner","session_key":"web:current"}]`,
		`[{"run_id":"failed","task_id":"current","owner_id":"foreign","session_key":"web:current"}]`,
		`[{"run_id":"failed","task_id":"current","owner_id":"owner","session_key":"web:foreign"}]`,
	} {
		db := fixture()
		db["accepted_inputs"] = json.RawMessage(row)
		if _, err = laba115ProjectionOwnedRun(db, task); err == nil {
			t.Fatal("uncorroborated run accepted")
		}
	}
	db := fixture()
	db["runs"] = json.RawMessage(`[{"run_id":"failed","task_id":"current","session_key":"web:current","task_revision":1,"constraints_revision":3,"current_state":"failed"}]`)
	if _, err = laba115ProjectionOwnedRun(db, task); err == nil {
		t.Fatal("stale task pin accepted")
	}
	db = fixture()
	db["runs"] = json.RawMessage(`[{"run_id":"failed","task_id":"current","session_key":"web:current","task_revision":2,"constraints_revision":3},{"run_id":"second","task_id":"current","session_key":"web:current","task_revision":2,"constraints_revision":3}]`)
	db["accepted_inputs"] = json.RawMessage(`[{"run_id":"failed","task_id":"current","owner_id":"owner","session_key":"web:current"},{"run_id":"second","task_id":"current","owner_id":"owner","session_key":"web:current"}]`)
	if _, err = laba115ProjectionOwnedRun(db, task); err == nil {
		t.Fatal("ambiguous run guessed")
	}
}
