package task_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

type controlProvider struct {
	started   chan transport.StartRunRequest
	cancelled chan string
	events    chan transport.TransportEvent
	final     bool
}

func (*controlProvider) Name() string { return "control-fixture" }
func (*controlProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *controlProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.started <- r
	if p.final {
		p.events <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: "draft only saved"})
		close(p.events)
	}
	return p.events, nil
}
func (*controlProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected continue")
}
func (*controlProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected tool result")
}
func (p *controlProvider) CancelRun(_ context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled <- r.RunID
	return nil, nil
}
func (f *controlsFixture) executor(p *controlProvider) *flow.Service {
	return flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), p, flow.Config{Tasks: f.tasks, Completion: completion.New(f.pool), ProviderName: p.Name(), OwnerID: "worker", LeaseTTL: 60}, nil)
}
func TestStopIdleProviderIgnoresLateFinalIntegration(t *testing.T) {
	f := newControlsFixture(t)
	p := &controlProvider{started: make(chan transport.StartRunRequest, 1), cancelled: make(chan string, 1), events: make(chan transport.TransportEvent, 1)}
	executor := f.executor(p)
	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(f.ctx, ingress.InputEvent{EventID: "old-search", IdempotencyKey: "old-search", SessionKey: f.session, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: fmt.Sprintf(`{"user_id":%q,"task_id":%q,"text":"search before sending"}`, f.owner, f.root.TaskID)})
		done <- err
	}()
	var request transport.StartRunRequest
	select {
	case request = <-p.started:
	case <-f.ctx.Done():
		t.Fatal("provider not started")
	}
	result, err := f.controls.Apply(f.actor("immediate-stop", "web"), task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: task.ControlStop})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Status != "cancelled" {
		t.Fatal("task not cancelled")
	}
	select {
	case <-p.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not cancelled promptly")
	}
	p.events <- transport.NewAssistantFinalEvent(request.Context.RunID, p.Name(), transport.AssistantFinal{Content: "LATE UNAUTHORIZED FINAL"})
	close(p.events)
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("revoked execution reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revoked execution did not stop")
	}
	var state string
	var late int
	if err = f.pool.QueryRow(f.ctx, `SELECT current_state FROM runs WHERE run_id=$1`, request.Context.RunID).Scan(&state); err != nil || state != "cancelled" {
		t.Fatal("late state overwrite", state, err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM messages WHERE content='LATE UNAUTHORIZED FINAL'`).Scan(&late); err != nil || late != 0 {
		t.Fatal("late transcript accepted", err)
	}
}
func TestAmendContinuationUsesCurrentProtectedContextIntegration(t *testing.T) {
	f := newControlsFixture(t)
	f.startRun(t, f.root.TaskID, "model_started")
	result, err := f.controls.Apply(f.actor("prepare", "web"), task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlAmend, Instruction: "Only prepare draft; do not send", ExecutionPolicy: task.PolicyPrepareOnly})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := durableinput.NewStore(f.pool).Get(f.ctx, result.ContinuationInputID)
	if err != nil {
		t.Fatal(err)
	}
	p := &controlProvider{started: make(chan transport.StartRunRequest, 1), cancelled: make(chan string, 1), events: make(chan transport.TransportEvent, 1), final: true}
	if _, err = f.executor(p).Execute(f.ctx, receipt.Event); err != nil {
		t.Fatal(err)
	}
	request := <-p.started
	var contextText string
	for _, item := range request.InputItems {
		if item.Role == "system" && strings.Contains(item.Content, "Butler task authority") {
			contextText = item.Content
		}
	}
	if !strings.Contains(contextText, "Only prepare draft; do not send") || !strings.Contains(contextText, "prepare_only") || !strings.Contains(contextText, `"revision":2`) {
		t.Fatal("current protected context missing", contextText)
	}
	current, err := f.tasks.Get(f.ctx, f.root.TaskID)
	if err != nil || current.Status != "active" || current.Revision != 2 {
		t.Fatal("model final completed or reverted amended task", err)
	}
}

type forbiddenExecutor struct{ calls int }

func (p *forbiddenExecutor) Execute(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error) {
	p.calls++
	return nil, errors.New("controls must not wait for model execution")
}
func TestOwnerIngressControlBypassesQueueAndAmbiguityNeverExecutesIntegration(t *testing.T) {
	f := newControlsFixture(t)
	f.startRun(t, f.root.TaskID, "awaiting_approval")
	forbidden := &forbiddenExecutor{}
	service := durableinput.NewService(durableinput.NewStore(f.pool), session.NewPostgresRepository(f.pool), forbidden, run.NewService(run.NewPostgresRepository(f.pool), nil), nil)
	service.SetTaskControls(f.controls)
	event := ingress.InputEvent{EventID: "stop-event", IdempotencyKey: "stop-event", SessionKey: f.session, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"стоп"}`}
	result, err := service.Execute(f.actor("stop-event", "web"), event)
	if err != nil || result.ReceiptStatus != "completed" || result.RunID != "" || forbidden.calls != 0 {
		t.Fatal("control enqueued", result, err)
	}
	replay, err := service.Execute(f.actor("stop-event", "web"), event)
	if err != nil || replay.InputID != result.InputID {
		t.Fatal("natural stop redelivery lost", err)
	}
	event.EventID = "ambiguous"
	event.IdempotencyKey = "ambiguous"
	waiting, err := service.Execute(f.actor("ambiguous", "web"), event)
	if err != nil || waiting.ReceiptStatus != "waiting_for_user" || waiting.RunID != "" || forbidden.calls != 0 {
		t.Fatal("no-task control guessed or executed", waiting, err)
	}
	event.EventID = "untrusted"
	event.IdempotencyKey = "untrusted"
	event.PayloadJSON = `{"intent":"stop","user_id":"controls-owner"}`
	if _, err = service.Execute(f.ctx, event); !errors.Is(err, task.ErrUnauthorized) {
		t.Fatal("untrusted control accepted", err)
	}
	event.EventID = "conversation"
	event.IdempotencyKey = "conversation"
	event.PayloadJSON = `{"intent":"conversation","text":"hello"}`
	conversation, err := service.Execute(f.actor("conversation", "web"), event)
	if err != nil || conversation.TaskID != "" || conversation.ReceiptStatus != "queued" {
		t.Fatal("conversation created or continued task", err)
	}
	var ambiguity string
	if err = f.pool.QueryRow(f.ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, waiting.InputID).Scan(&ambiguity); err != nil || ambiguity != "cancelled" {
		t.Fatal("explicit conversation left ambiguous input head", err)
	}
}
