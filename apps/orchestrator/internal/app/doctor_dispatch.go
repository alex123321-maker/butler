package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
)

type operatorDoctorChecker struct{ deps httpServerDeps }

func (c *operatorDoctorChecker) RunCheck(ctx context.Context) (string, json.RawMessage, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.Channel != "web" {
		return "unknown", nil, fmt.Errorf("authenticated owner required")
	}
	id, err := budget.NewOperationID()
	if err != nil {
		return "unknown", nil, err
	}
	// Real session and task identities come from their owning services. A fresh
	// session keeps a diagnostic waiting for approval from blocking normal input.
	key := "web:doctor:" + id
	a.SessionKey = key
	a.DeliveryID = id
	ctx = owneridentity.WithActor(ctx, a)
	_, _, err = c.deps.sessionRepo.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: a.OwnerID, Channel: "web", MetadataJSON: `{"operator":"doctor"}`})
	if err != nil {
		return "unknown", nil, err
	}
	tx, err := c.deps.postgres.Pool().Begin(ctx)
	if err != nil {
		return "unknown", nil, err
	}
	defer tx.Rollback(ctx)
	t, err := c.deps.taskStore.CreateTx(ctx, tx, task.CreateRequest{SessionKey: key, OwnerID: a.OwnerID, Goal: "Run doctor.check_system through Tool Broker and retain its diagnostic result", Constraints: []string{"Only doctor.check_system with empty arguments; no model calls"}, CompletionCriteria: []string{"A Broker-authorized diagnostic observation is recorded"}, Source: map[string]any{"type": "operator_doctor", "channel": "web"}})
	if err != nil {
		return "unknown", nil, err
	}
	payload, _ := json.Marshal(map[string]any{"text": "Run system diagnostics", "user_id": a.OwnerID, "task_id": t.TaskID})
	e := ingress.InputEvent{EventID: id, IdempotencyKey: id, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: key, Source: "web", PayloadJSON: string(payload), CreatedAt: time.Now().UTC()}
	r, err := c.deps.inputStore.AcceptDoctorTx(ctx, tx, e, a.OwnerID, t.TaskID)
	if err != nil {
		return "unknown", nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "unknown", nil, err
	}
	return "unknown", pendingJSON(r), nil
}
func pendingJSON(r durableinput.Record) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"status": "unknown", "checks": []any{}, "input_id": r.InputID, "execution": map[string]any{"status": r.Status, "run_id": r.RunID, "task_id": r.TaskID, "session_key": r.SessionKey}})
	return b
}
func (c *operatorDoctorChecker) ReadCheck(ctx context.Context, id string) (string, json.RawMessage, error) {
	r, err := c.deps.inputStore.GetDoctor(ctx, id)
	if err != nil {
		return "unknown", nil, err
	}
	if r.Status == "queued" || r.Status == "running" || r.Status == "waiting_for_user" {
		return "unknown", pendingJSON(r), nil
	}
	if r.Status != "completed" {
		raw := doctorExecutionFailure(r)
		var report struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(raw, &report)
		return report.Status, raw, nil
	}
	// Final results remain in their owning store; transcript text is not used as
	// authority and arbitrary event payloads cannot fabricate this observation.
	result, err := c.deps.finalResults.GetByRun(ctx, r.RunID)
	if err != nil {
		return "unknown", nil, err
	}
	if result.PrivacyRedacted {
		return "unknown", nil, fmt.Errorf("diagnostic result is no longer available")
	}
	checker := apiservice.NewToolBrokerDoctorChecker(func(context.Context, string, string) (string, error) { return result.Content, nil })
	status, raw, err := checker.RunCheck(ctx)
	var body map[string]any
	if json.Unmarshal(raw, &body) == nil {
		body["input_id"] = r.InputID
		body["execution"] = map[string]any{"status": r.Status, "run_id": r.RunID, "task_id": r.TaskID, "session_key": r.SessionKey}
		raw, _ = json.Marshal(body)
	}
	return status, raw, err
}
func doctorExecutionFailure(r durableinput.Record) json.RawMessage {
	reason := "execution_unavailable"
	status := "unknown"
	switch r.ErrorCode {
	case "broker_transport":
		reason = "transport_failed"
	case "preparation_denied", "authorization_denied":
		reason = "authorization_denied"
		status = "degraded"
	case "effect_unknown":
		reason = "dispatch_outcome_unknown"
	case "execution_deadline_exceeded":
		reason = "timeout"
	}
	b, _ := json.Marshal(map[string]any{"status": status, "check_error": reason, "input_id": r.InputID, "execution": map[string]any{"status": r.Status, "run_id": r.RunID, "task_id": r.TaskID}, "checks": []any{map[string]any{"name": "broker_execution", "status": status, "message": "execution stopped; inspect the owner task and approval"}}})
	return b
}
