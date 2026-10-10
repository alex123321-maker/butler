package goal_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type idleProvider struct {
	started   chan transport.StartRunRequest
	cancelled chan string
	events    chan transport.TransportEvent
}

func (*idleProvider) Name() string { return "synthetic-provider" }
func (*idleProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *idleProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.started <- r
	return p.events, nil
}
func (*idleProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected continue")
}
func (*idleProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected tools")
}
func (p *idleProvider) CancelRun(_ context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled <- r.RunID
	return nil, nil
}

func TestGoalRevocationCancelsIdleProviderBeforeSweepAndIgnoresLateFinalIntegration(t *testing.T) {
	f := setup(t)
	r := f.create(t, "task")
	p := &idleProvider{started: make(chan transport.StartRunRequest, 1), cancelled: make(chan string, 1), events: make(chan transport.TransportEvent, 1)}
	executor := flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), p, flow.Config{Tasks: f.tasks, OwnerGoals: f.manager, Completion: completion.New(f.pool), ProviderName: p.Name(), OwnerID: "worker", LeaseTTL: 60}, nil)
	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(f.ctx, ingress.InputEvent{EventID: "goal-input", IdempotencyKey: "goal-input", SessionKey: f.key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: fmt.Sprintf(`{"user_id":%q,"task_id":%q,"text":"diagnose test service"}`, f.owner, r.TaskID)})
		done <- err
	}()
	var request transport.StartRunRequest
	select {
	case request = <-p.started:
	case <-time.After(5 * time.Second):
		select {
		case err := <-done:
			t.Fatal("provider admission failed", err)
		default:
			t.Fatal("provider did not start")
		}
	}
	protected := false
	for _, item := range request.InputItems {
		if strings.Contains(item.Content, "Confirmed owner goal") {
			origin := content.ItemOrigin(item)
			protected = item.Role == "system" && origin.Kind == content.OwnerPolicy && len(origin.SourceRefs) >= 2 && strings.Contains(item.Content, "Keep work service out of scope") && strings.Contains(item.Content, f.m.ID)
		}
	}
	if !protected {
		t.Fatal("full typed owner policy context unavailable")
	}
	revoked := f.g
	revoked.Status = "revoked"
	if _, err := f.manager.PutGoal(f.actor("revoke-idle-goal"), goal.GoalRequest{Goal: revoked, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	// No propagation worker is running. Authority supervision alone must cancel
	// model transport promptly after the durable authority-row commit.
	select {
	case <-p.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("idle provider survived revoked goal before sweep")
	}
	p.events <- transport.NewAssistantFinalEvent(request.Context.RunID, p.Name(), transport.AssistantFinal{Content: "LATE GOAL FINAL"})
	close(p.events)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("revoked run reported success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("late stream did not terminate")
	}
	for _, table := range []string{"run_final_results", "delivery_outbox", "task_results"} {
		var count int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE run_id=$1`, request.Context.RunID).Scan(&count); err != nil || count != 0 {
			t.Fatal("late result accepted", table, count, err)
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM messages WHERE content='LATE GOAL FINAL'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("late final entered transcript", count, err)
	}
}
