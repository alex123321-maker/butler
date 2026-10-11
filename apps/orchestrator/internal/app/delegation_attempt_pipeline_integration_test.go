package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

type parentAttemptProbe struct {
	transport.ModelProvider
	starts         int
	childID        string
	failedRevision int64
}

func (*parentAttemptProbe) Name() string                    { return "fixture" }
func (*parentAttemptProbe) InferenceLimits() (int64, int64) { return 64, 16 }
func (*parentAttemptProbe) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (p *parentAttemptProbe) StartRun(_ context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	found := false
	for _, item := range request.InputItems {
		if strings.Contains(item.Content, "SECRET_DIAGNOSTIC_MARKER") {
			return nil, errors.New("raw provider error reached parent")
		}
		if marked, _ := item.Metadata["butler_failed_delegation_attempt"].(bool); !marked {
			continue
		}
		var wrapped struct {
			Data delegation.AttemptReport `json:"data"`
		}
		origin := content.ItemOrigin(item)
		if found || json.Unmarshal([]byte(item.Content), &wrapped) != nil || item.Role != "user" || origin.IsInstruction() || origin.Kind != content.SubagentOutput || origin.Sensitivity != content.Private || len(origin.SourceRefs) == 0 || wrapped.Data.TaskID != p.childID || wrapped.Data.TaskStatus != "blocked" || wrapped.Data.TaskRevision != p.failedRevision+1 || len(wrapped.Data.Attempts) != 1 || wrapped.Data.Attempts[0].TaskRevision != p.failedRevision || len(wrapped.Data.ModelUsage) != 2 || wrapped.Data.ModelUsage[1].InputTokens != nil || len(wrapped.Data.Actions) != 1 || wrapped.Data.Actions[0].TaskRevision != p.failedRevision || wrapped.Data.Actions[0].State != "effect_unknown" {
			return nil, errors.New("failed-attempt DATA projection changed at provider boundary")
		}
		found = true
	}
	if !found {
		return nil, errors.New("current parent did not receive a failed-attempt observation")
	}
	events := make(chan transport.TransportEvent, 2)
	events <- transport.NewAssistantFinalEvent(request.Context.RunID, "fixture", transport.AssistantFinal{Content: "Child attempt failed; independent verification is still required.", FinishReason: "completed"})
	events <- transport.NewTerminalEvent(request.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	close(events)
	return events, nil
}
func (*parentAttemptProbe) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

type ownedAttemptReaderFunc func(context.Context, string) ([]delegation.AttemptReport, error)

func (f ownedAttemptReaderFunc) LoadFailedDelegationAttempts(ctx context.Context, id string) ([]delegation.AttemptReport, error) {
	return f(ctx, id)
}

// Inference is scripted; durable ingress, run/task/lease, source release guard,
// branch/root budget reservations and final persistence are real components.
func TestDelegationFailedAttemptActualParentPipelineIntegration(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		name := "current-report"
		if deleted {
			name = "deleted-after-preparation"
		}
		t.Run(name, func(t *testing.T) {
			f, c, child, _ := failedDelegationFixture(t, "SECRET_DIAGNOSTIC_MARKER password=fixture-private", true)
			if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='blocked',error_code='fixture_failed' WHERE task_id=$1`, child.TaskID); err != nil {
				t.Fatal(err)
			}
			inputs := durableinput.NewStore(f.pool)
			payload, _ := json.Marshal(map[string]any{"task_id": f.parent.TaskID, "text": "Observe the failed child and preserve uncertainty"})
			event := ingress.InputEvent{EventID: "observe-parent", IdempotencyKey: "observe-parent", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: f.parent.SessionKey, Source: "web", PayloadJSON: string(payload), CreatedAt: time.Now().UTC()}
			receipt, _, err := inputs.Accept(f.ctx, event, f.parent.OwnerID, f.parent.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := inputs.Claim(f.ctx, time.Minute)
			if err != nil || claim.InputID != receipt.InputID {
				t.Fatal("current parent receipt", err)
			}
			reader := failedDelegationAttemptReader{pool: f.pool, manager: f.manager}
			reads := 0
			ownedReader := ownedAttemptReaderFunc(func(ctx context.Context, id string) ([]delegation.AttemptReport, error) {
				reports, err := reader.LoadFailedDelegationAttempts(ctx, id)
				if err == nil && len(reports) == 1 && reports[0].TaskID == child.TaskID {
					reads++
				}
				if err == nil && deleted {
					_, err = f.sources.Delete(f.owner, c.ContractSource.ID, "delete", c.ContractSource.Revision)
				}
				return reports, err
			})
			provider := &parentAttemptProbe{childID: child.TaskID, failedRevision: child.Revision}
			results := completion.New(f.pool)
			results.DataGuard = func(ctx context.Context, tx pgx.Tx, id string) error { return f.sources.CheckRunSourcesTx(ctx, tx, id) }
			executor := flow.NewService(session.NewPostgresRepository(f.pool), f.leases, runservice.NewService(runservice.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "attempt-parent-worker", AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1, Tasks: f.store, Checkpoints: inputs, Completion: results, MemorySources: f.sources, Budgets: f.budgets, DelegationContexts: delegatedContextReader{sources: f.sources, manager: f.manager}, FailedDelegationAttempts: ownedReader}, nil)
			output, err := executor.Execute(durableinput.WithClaim(f.ctx, claim), claim.Event)
			if reads != 1 {
				t.Fatal("parent pipeline did not reach the actual observation boundary", err, reads)
			}
			if deleted {
				if provider.starts != 0 || err == nil && (output == nil || output.CurrentState == commonv1.RunState_RUN_STATE_COMPLETED) {
					t.Fatal("deleted observation crossed the provider boundary", err, output, provider.starts)
				}
				return
			}
			if err != nil || output == nil || output.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || provider.starts != 1 {
				t.Fatal("actual parent pipeline", err, output, provider.starts)
			}
			var checkpoint, status string
			if err = f.pool.QueryRow(f.ctx, `SELECT checkpoint_json::text FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&checkpoint); err != nil || strings.Contains(checkpoint, "SECRET_DIAGNOSTIC_MARKER") || strings.Contains(checkpoint, "attempt-action:") {
				t.Fatal("durable checkpoint copied an observation body", err)
			}
			if err = f.pool.QueryRow(f.ctx, `SELECT status FROM tasks WHERE task_id=$1`, child.TaskID).Scan(&status); err != nil || status != "blocked" {
				t.Fatal("observation unblocked/completed child", err, status)
			}
			var parentAttempts int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_model_attempts WHERE run_id=$1`, output.RunID).Scan(&parentAttempts); err != nil || parentAttempts != 1 {
				t.Fatal("parent turn lacked a durable shared budget reservation", err, parentAttempts)
			}
		})
	}
}
