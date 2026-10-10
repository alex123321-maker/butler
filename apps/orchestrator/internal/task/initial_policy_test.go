package task_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func TestInitialExecutionPolicyIsPrivateAndValidated(t *testing.T) {
	var req task.CreateRequest
	if err := json.Unmarshal([]byte(`{"initial_execution_policy":"normal","InitialExecutionPolicy":"normal","session_key":"web:test","owner_id":"test","goal":"prepare","completion_criteria":["reviewed"]}`), &req); err != nil || req.InitialExecutionPolicy != "" {
		t.Fatal("JSON supplied trusted policy", req, err)
	}
	req.InitialExecutionPolicy = task.PolicyPrepareOnly
	encoded, err := json.Marshal(req)
	if err != nil || strings.Contains(string(encoded), "prepare_only") {
		t.Fatal("private policy escaped ingress contract", string(encoded), err)
	}
	if err = task.ValidateCreate(req); err != nil {
		t.Fatal(err)
	}
	req.InitialExecutionPolicy = "unbounded"
	if err = task.ValidateCreate(req); !errors.Is(err, task.ErrInvalid) {
		t.Fatal("unknown trusted policy accepted", err)
	}
}
