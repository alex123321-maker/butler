package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

type resumedProvider struct {
	f          *fixture
	starts     int
	quietFinal bool
}

func (*resumedProvider) Name() string { return "fixture" }
func (*resumedProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *resumedProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	if q.Context.ProviderSessionRef != nil {
		return nil, errors.New("old provider context reused")
	}
	data := false
	guidance := false
	for _, item := range q.InputItems {
		if content.ItemOrigin(item).Kind == content.KernelInstruction && strings.Contains(item.Content, "question-asking step is complete") {
			guidance = true
		}
		if strings.Contains(item.Content, `"kind":"interaction_response"`) {
			if item.Role != "user" || content.ItemOrigin(item).Kind != content.ToolOutput {
				return nil, errors.New("preference became instruction")
			}
			data = strings.Contains(item.Content, `"option_ids":["a"]`)
		}
	}
	if !data || !guidance {
		return nil, errors.New("durable answer absent from reconstructed context")
	}
	// Test owning publication directly here. Exact Broker/private HTTP proof is
	// exercised independently in app's real dispatched-control integration test.
	if !p.quietFinal {
		tx, e := p.f.pool.Begin(ctx)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback(ctx)
		if _, e = p.f.s.ApplyTx(ctx, tx, p.f.p, communicationcontrol.Envelope{RunID: q.Context.RunID, ToolCallID: "resumed-result", EffectID: "resumed:" + q.Context.RunID, Tool: "communication.message", ArgsJSON: message}); e != nil {
			return nil, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, e
		}
	}
	ch := make(chan transport.TransportEvent, 2)
	ch <- transport.NewAssistantFinalEvent(q.Context.RunID, "fixture", transport.AssistantFinal{FinishReason: "stop"})
	ch <- transport.NewTerminalEvent(q.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	close(ch)
	return ch, nil
}
func (*resumedProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected correction")
}
func (*resumedProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("old tool redispatched")
}
func (*resumedProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func TestCommunicationActualCoreFreshContextResumeIntegration(t *testing.T) {
	for _, phase := range []string{"awaiting_approval", "model_running", "quiet", "verified_task"} {
		t.Run(phase, func(t *testing.T) {
			quiet := phase == "quiet" || phase == "verified_task"
			f := setup(t, fixtureOptions{Task: true, Quiet: phase == "quiet", VerifiedTask: phase == "verified_task"})
			job, e := task.NewStore(f.pool).Get(f.ctx, f.p.TaskID)
			if e != nil {
				t.Fatal(e)
			}
			prepared, _ := json.Marshal(map[string]any{"Task": job})
			if e = f.s.Inputs.SaveCheckpoint(f.ctx, f.p.RunID, "prepared", string(prepared)); e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.SaveCheckpoint(f.ctx, f.p.RunID, "tool_dispatching", `{}`); e != nil {
				t.Fatal(e)
			}
			r, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.Respond(f.actor("telegram", "answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
				t.Fatal(e)
			}
			if _, e = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); e != nil {
				t.Fatal(e)
			}
			claim, e := f.s.Inputs.Claim(f.ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			if phase == "model_running" {
				// A crash after activating the Task/provider phase but before any new
				// action checkpoint must safely reconstruct the same preference.
				if _, e = f.pool.Exec(f.ctx, `UPDATE runs SET current_state='model_running',status='model_running' WHERE run_id=$1`, f.p.RunID); e != nil {
					t.Fatal(e)
				}
				if _, e = f.pool.Exec(f.ctx, `UPDATE tasks SET status='active' WHERE task_id=$1`, f.p.TaskID); e != nil {
					t.Fatal(e)
				}
			}
			provider := &resumedProvider{f: f, quietFinal: quiet}
			writer := completion.New(f.pool)
			writer.Communication = f.s
			core := flow.NewService(session.NewPostgresRepository(f.pool), f.leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{Communication: f.s, Completion: writer, Checkpoints: f.s.Inputs, OwnerID: "resumed-worker", LeaseTTL: 30, ProviderName: "fixture", ModelName: "fixture"}, nil)
			result, e := core.Resume(input.WithClaim(context.Background(), claim), f.p.RunID, ingress.InputEvent{SessionKey: f.p.SessionKey, Source: "fixture", PayloadJSON: `{"text":"ask owner"}`})
			if e != nil || result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || provider.starts != 1 {
				t.Fatal(result, e, provider.starts)
			}
			pubs, msgs, deliveries := 2, 4, 4
			if quiet {
				pubs, msgs, deliveries = 1, 3, 2
			}
			if f.count(t, "communication_publications") != pubs || f.count(t, "messages") != msgs || f.count(t, "delivery_outbox") != deliveries {
				t.Fatal("resume duplicated publication/response/final")
			}
		})
	}
}
