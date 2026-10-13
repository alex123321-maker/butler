package input

import (
	"context"
	"encoding/json"
	"errors"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

type modelRetryCurrentSource struct{ pool *pgxpool.Pool }

func (*modelRetryCurrentSource) PinTranscript(context.Context, string, int64) error { return nil }
func (s *modelRetryCurrentSource) CheckRunSources(ctx context.Context, _ string) error {
	var active bool
	if s.pool.QueryRow(ctx, `SELECT active FROM model_retry_test_current_source`).Scan(&active) != nil || !active {
		return errors.New("current source revoked")
	}
	return nil
}

type modelRetryWorkerTools struct {
	pool            *pgxpool.Pool
	calls, prepares int
}

func (*modelRetryWorkerTools) ListTools(context.Context) ([]*toolbrokerv1.ToolContract, error) {
	return []*toolbrokerv1.ToolContract{{ToolName: "http.request", Status: "active", InputSchemaJson: `{"type":"object"}`, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe"}}}, nil
}
func (p *modelRetryWorkerTools) PrepareAction(ctx context.Context, c *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	p.prepares++
	var taskID, owner string
	var pinned, revision, constraints int64
	if err := p.pool.QueryRow(ctx, `SELECT r.task_id,s.user_id,r.task_revision,t.revision,t.constraints_revision FROM runs r JOIN sessions s USING(session_key) JOIN tasks t USING(task_id) WHERE r.run_id=$1`, c.GetRunId()).Scan(&taskID, &owner, &pinned, &revision, &constraints); err != nil {
		return actionauth.PreparedAction{}, err
	}
	contract := &toolbrokerv1.ToolContract{ToolName: c.GetToolName(), Status: "active", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{}`}
	b, _, err := actionauth.BuildBinding(actionauth.StableActionID(c.GetRunId(), c.GetToolCallId()), c, contract, actionauth.ContextSnapshot{RunID: c.GetRunId(), OwnerID: owner, TaskID: taskID, PinnedTaskRevision: pinned, TaskRevision: revision, ConstraintsRevision: constraints})
	return actionauth.PreparedAction{Binding: b}, err
}
func (*modelRetryWorkerTools) AuthorizeAction(context.Context, *toolbrokerv1.ToolCall, string, string, string) (string, error) {
	return "synthetic-private-grant", nil
}
func (*modelRetryWorkerTools) ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	return nil, errors.New("private dispatch required")
}
func (p *modelRetryWorkerTools) ExecuteAuthorizedToolCall(_ context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	if grant == "" {
		return nil, errors.New("grant absent")
	}
	p.calls++
	return &toolbrokerv1.ToolResult{RunId: c.GetRunId(), ToolCallId: c.GetToolCallId(), ToolName: c.GetToolName(), Status: "completed", ResultJson: `{"receipt":"confirmed-once"}`}, nil
}

type modelRetryWorkerProvider struct {
	calls                           []transport.StartRunRequest
	tool, duplicate                 bool
	submits                         int
	stage                           *transport.ProviderStage
	hold, failAgain, cancelTerminal bool
	cancellations                   int
	startWait, active               bool
}

func (*modelRetryWorkerProvider) Name() string                    { return "fixture" }
func (*modelRetryWorkerProvider) InferenceLimits() (int64, int64) { return 64, 8 }
func (*modelRetryWorkerProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsToolCalls: true}, nil
}
func (p *modelRetryWorkerProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	stage := transport.ProviderStageHTTP
	if p.stage != nil {
		stage = *p.stage
	}
	transport.ObserveProviderStage(ctx, stage)
	p.calls = append(p.calls, r)
	p.active = true
	if p.startWait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	out := make(chan transport.TransportEvent, 2)
	if p.hold {
		return out, nil
	}
	if p.cancelTerminal {
		out <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCancelled, "fixture")
		close(out)
		return out, nil
	}
	known := false
	for _, item := range r.InputItems {
		if content.ItemOrigin(item).Kind == content.ToolOutput && item.Role == "user" {
			var b struct {
				Data struct {
					Already bool `json:"already_executed"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(item.Content), &b) == nil && b.Data.Already {
				known = true
			}
		}
	}
	if (len(p.calls) == 1 || p.failAgain) && !p.tool {
		out <- transport.NewTransportErrorEvent(r.Context.RunID, "fixture", &transport.Error{Type: transport.ErrorTypeProviderTimeout})
		close(out)
		return out, nil
	}
	usage := &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	if p.tool && (!known || p.duplicate) {
		id, args := "confirmed-call", `{"method":"POST","url":"https://fixture.invalid/one","amount":1}`
		if known {
			id, args = "NEW-CALL-ID", `{"amount":1.00,"url":"https://fixture.invalid/one","method":"POST"}`
		}
		e := transport.NewToolCallRequestedEvent(r.Context.RunID, "fixture", transport.ToolCallRequest{ToolCallRef: id, ToolName: "http.request", ArgsJSON: args})
		e.Usage = usage
		out <- e
	} else {
		out <- transport.NewAssistantFinalEvent(r.Context.RunID, "fixture", transport.AssistantFinal{Content: "known result accepted", FinishReason: "STOP", Usage: usage})
		out <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	}
	close(out)
	return out, nil
}
func (*modelRetryWorkerProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected continuation")
}
func (p *modelRetryWorkerProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	transport.ObserveProviderStage(ctx, transport.ProviderStageHTTP)
	p.submits++
	out := make(chan transport.TransportEvent, 1)
	out <- transport.NewTransportErrorEvent(r.RunID, "fixture", &transport.Error{Type: transport.ErrorTypeProviderTimeout})
	close(out)
	return out, nil
}
func (p *modelRetryWorkerProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancellations++
	p.active = false
	return nil, nil
}

type modelRetryWorkerFixture struct {
	pool        *pgxpool.Pool
	store       *Store
	worker      *Service
	provider    *modelRetryWorkerProvider
	tools       *modelRetryWorkerTools
	receipt     Record
	sessionRepo *session.PostgresRepository
	runs        *run.Service
	tasks       *task.Store
	sources     *modelRetryCurrentSource
	policy      budget.Policy
	transcript  *transcript.Store
}

func newModelRetryWorkerFixture(t *testing.T, tool, duplicate bool) *modelRetryWorkerFixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	store := NewStore(pool)
	sessions := session.NewPostgresRepository(pool)
	if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "model-retry-session", UserID: "owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(pool)
	job, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "model-retry-session", OwnerID: "owner", Goal: "one confirmed action", CompletionCriteria: []string{"one receipt"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE model_retry_test_current_source(active boolean NOT NULL);INSERT INTO model_retry_test_current_source VALUES(true)`); err != nil {
		t.Fatal(err)
	}
	event := ingress.InputEvent{EventID: "model-retry-input", IdempotencyKey: "model-retry-input", SessionKey: "model-retry-session", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"complete one action","user_id":"owner","task_id":"` + job.TaskID + `"}`, CreatedAt: time.Now().UTC()}
	receipt, _, err := store.Accept(ctx, event, "owner", job.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	p := budget.DefaultPolicy()
	p.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	p.OutputTokens = 8
	p.ModelMillis = 3000
	f := &modelRetryWorkerFixture{pool: pool, store: store, provider: &modelRetryWorkerProvider{tool: tool, duplicate: duplicate}, tools: &modelRetryWorkerTools{pool: pool}, receipt: receipt, sessionRepo: sessions, runs: run.NewService(run.NewPostgresRepository(pool), nil), tasks: tasks, sources: &modelRetryCurrentSource{pool: pool}, policy: p, transcript: transcript.NewStore(pool)}
	f.restart()
	return f
}
func (f *modelRetryWorkerFixture) restart() {
	executor := flow.NewService(f.sessionRepo, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, f.provider, flow.Config{Completion: completion.New(f.pool), ProviderName: "fixture", ModelName: "fixture", OwnerID: "retry-worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: NewStore(f.pool), Tools: f.tools, ActionAuthorizer: f.tools, ToolCatalog: f.tools, MemorySources: f.sources, Budgets: budget.NewController(f.pool, f.policy)}, nil)
	f.worker = NewService(NewStore(f.pool), f.sessionRepo, executor, f.runs, nil)
}
func (f *modelRetryWorkerFixture) step(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	claim, err := f.store.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal("claim", err)
	}
	f.worker.process(ctx, claim)
}
func (f *modelRetryWorkerFixture) result(t *testing.T) Record {
	t.Helper()
	r, err := f.store.Get(context.Background(), f.receipt.InputID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestModelRetryWorkerFreshSessionNoRepeatedEffectIntegration(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tool, duplicate bool
		models, effects int
		status          string
	}{{"initial_timeout_two_reservations", false, false, 2, 0, "completed"}, {"confirmed_tool_three_reservations", true, false, 3, 1, "completed"}, {"new_id_equivalent_write_denied", true, true, 3, 1, "blocked"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newModelRetryWorkerFixture(t, tc.tool, tc.duplicate)
			f.step(t)
			r := f.result(t)
			if r.Status != "queued" || r.Checkpoint != "model_retry_pending" {
				t.Fatal("nominal timeout terminalized or blocked task instead of queued retry", r.Status, r.Checkpoint, r.ErrorMessage)
			}
			time.Sleep(2100 * time.Millisecond)
			f.restart()
			f.step(t)
			r = f.result(t)
			var models, unknown int
			if f.pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE actual_input_tokens IS NULL AND actual_output_tokens IS NULL) FROM execution_model_attempts WHERE run_id=$1`, r.RunID).Scan(&models, &unknown) != nil || models != tc.models || unknown != 1 || r.Status != tc.status || f.tools.calls != tc.effects || len(f.provider.calls) != 2 || f.provider.calls[1].Context.ProviderSessionRef != nil {
				t.Fatal("spent/NULL models, fresh session, or one-effect invariant changed", models, unknown, r.Status, f.tools.calls, r.ErrorMessage)
			}
			var count, state string
			if f.pool.QueryRow(context.Background(), `SELECT checkpoint_json->'model_retry'->>'count',checkpoint_json->'model_retry'->>'state' FROM accepted_inputs WHERE input_id=$1`, r.InputID).Scan(&count, &state) != nil || count != "1" || state != "started" {
				t.Fatal("durable count missing after restart")
			}
			if tc.duplicate && f.tools.prepares != 1 {
				t.Fatal("new-ID equivalent write reached authorizer before denial")
			}
		})
	}
}

func TestModelRetryWorkerStopAndSourceDuringDelayIntegration(t *testing.T) {
	for _, name := range []string{"stop_during_delay", "source_revoked_during_delay", "budget_exhausted_during_delay"} {
		t.Run(name, func(t *testing.T) {
			f := newModelRetryWorkerFixture(t, false, false)
			if name == "budget_exhausted_during_delay" {
				f.policy.Task.ModelTurns = 1
				f.restart()
			}
			f.step(t)
			r := f.result(t)
			if r.Status != "queued" {
				t.Fatal("fixture did not schedule retry")
			}
			switch name {
			case "stop_during_delay":
				leases := session.NewPostgresLeaseManager(f.pool)
				controller := task.NewController(f.tasks, leases.Authority(), leases, run.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), f.store, f.transcript)
				actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", SessionKey: "model-retry-session", Channel: "web", DeliveryID: "stop-model-retry"})
				if _, err := controller.Apply(actor, task.ControlRequest{TaskID: r.TaskID, ExpectedRevision: 1, Kind: task.ControlStop}); err != nil {
					t.Fatal("current owner Stop failed", err)
				}
				if f.result(t).Status != "cancelled" {
					t.Fatal("Stop did not cancel the queued retry receipt")
				}
				if _, err := f.store.Claim(context.Background(), time.Minute); !errors.Is(err, ErrNotFound) {
					t.Fatal("stopped retry remained claimable")
				}
				if len(f.provider.calls) != 1 {
					t.Fatal("Stop caused another provider request")
				}
				return
			case "source_revoked_during_delay":
				if _, err := f.pool.Exec(context.Background(), `UPDATE model_retry_test_current_source SET active=false`); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(2100 * time.Millisecond)
			f.restart()
			f.step(t)
			r = f.result(t)
			if r.Status != "blocked" || len(f.provider.calls) != 1 {
				t.Fatal("Stop/current source/budget denial admitted a new provider call", r.Status, r.ErrorMessage)
			}
		})
	}
}

func TestModelRetryWorkerUnsafeTimeoutNeverQueuesIntegration(t *testing.T) {
	for _, name := range []string{"absent_phase", "authentication_phase", "serialization_phase", "caller_deadline", "scope_deadline", "cancelled_terminal"} {
		t.Run(name, func(t *testing.T) {
			f := newModelRetryWorkerFixture(t, false, false)
			switch name {
			case "absent_phase":
				stage := transport.ProviderStage("")
				f.provider.stage = &stage
				f.provider.hold = true
			case "authentication_phase":
				stage := transport.ProviderStageAuthentication
				f.provider.stage = &stage
				// Hang inside StartRun rather than returning an idle stream.
				f.provider.startWait = true
			case "serialization_phase":
				stage := transport.ProviderStageSerialization
				f.provider.stage = &stage
				f.provider.hold = true
			case "caller_deadline":
				f.provider.hold = true
				f.worker.executionTimeout = time.Second
			case "scope_deadline":
				f.provider.hold = true
				f.policy.TaskLifetimeMillis = 1100
				f.restart()
			case "cancelled_terminal":
				f.provider.cancelTerminal = true
			}
			if name == "absent_phase" || name == "authentication_phase" || name == "serialization_phase" {
				// Exercise the owning nominal deadline rather than a provider's
				// synthetic timeout event: no observed inference phase means the
				// existing budget stop must cancel, retain NULL usage and not retry.
				f.policy.ModelMillis = 1000
				f.restart()
			}
			f.step(t)
			r := f.result(t)
			if r.Status == "queued" || len(f.provider.calls) != 1 {
				t.Fatal("unsafe timeout queued automatic model work", r.Status, r.ErrorMessage, len(f.provider.calls))
			}
			if name == "absent_phase" || name == "authentication_phase" || name == "serialization_phase" {
				if r.Status != "blocked" || r.ErrorCode != "execution_budget_deadline" || f.provider.cancellations != 1 || f.provider.active {
					t.Fatal("unobserved inference deadline bypassed budget stop/cancel", r.Status, r.ErrorCode, f.provider.cancellations)
				}
			}
			var hasRetry bool
			var count, unknown int
			if f.pool.QueryRow(context.Background(), `SELECT checkpoint_json ? 'model_retry' FROM accepted_inputs WHERE input_id=$1`, r.InputID).Scan(&hasRetry) != nil || hasRetry || f.pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE actual_input_tokens IS NULL AND actual_output_tokens IS NULL) FROM execution_model_attempts WHERE run_id=$1`, r.RunID).Scan(&count, &unknown) != nil || count != 1 || unknown != 1 {
				t.Fatal("unsafe model timeout fabricated a retry marker or refunded reservation")
			}
		})
	}
}

type modelRetryCrashAfterResume struct {
	base   *flow.Service
	cancel context.CancelFunc
}

func (p modelRetryCrashAfterResume) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	return p.base.Execute(ctx, event)
}
func (p modelRetryCrashAfterResume) Resume(ctx context.Context, runID string, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	result, err := p.base.Resume(ctx, runID, event)
	p.cancel()
	return result, err
}

func TestModelRetryWorkerSecondTimeoutStopsIntegration(t *testing.T) {
	f := newModelRetryWorkerFixture(t, false, false)
	f.provider.failAgain = true
	f.step(t)
	if f.result(t).Status != "queued" || f.provider.cancellations != 1 || f.provider.active {
		t.Fatal("first nominal timeout did not queue one retry")
	}
	time.Sleep(2100 * time.Millisecond)
	f.restart()
	base, ok := f.worker.executor.(*flow.Service)
	if !ok {
		t.Fatal("ordinary Core fixture missing")
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.worker.executor = modelRetryCrashAfterResume{base: base, cancel: cancel}
	claim, err := f.store.Claim(parent, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	f.worker.process(parent, claim)
	r := f.result(t)
	if r.Status != "running" || len(f.provider.calls) != 2 || parent.Err() == nil || f.provider.cancellations != 2 || f.provider.active {
		t.Fatal("post-second-timeout process loss did not leave exact durable retry receipt")
	}
	var stoppedStage, stoppedJSON string
	var stopped struct {
		Data struct {
			Reason string `json:"reason"`
		} `json:"checkpoint_data"`
	}
	if stoppedStage, stoppedJSON, err = f.store.LoadCheckpoint(context.Background(), r.RunID); err != nil || stoppedStage != "execution_stopped" || json.Unmarshal([]byte(stoppedJSON), &stopped) != nil || stopped.Data.Reason != "model_turn_timeout" {
		t.Fatal("exhausted model retry bypassed budget stop/cancel", stoppedStage, stoppedJSON, err)
	}
	time.Sleep(1100 * time.Millisecond)
	f.restart()
	f.step(t)
	r = f.result(t)
	var count, unknown int
	var retryCount, state string
	if f.pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE actual_input_tokens IS NULL AND actual_output_tokens IS NULL) FROM execution_model_attempts WHERE run_id=$1`, r.RunID).Scan(&count, &unknown) != nil || count != 2 || unknown != 2 || len(f.provider.calls) != 2 || r.Status != "blocked" || r.Attempts != 3 || f.provider.cancellations != 2 || f.provider.active {
		t.Fatal("restart reopened the started retry or changed spent NULLs", count, unknown, r.Status, r.Attempts, len(f.provider.calls))
	}
	if f.pool.QueryRow(context.Background(), `SELECT checkpoint_json->'model_retry'->>'count',checkpoint_json->'model_retry'->>'state' FROM accepted_inputs WHERE input_id=$1`, r.InputID).Scan(&retryCount, &state) != nil || retryCount != "1" || state != "started" {
		t.Fatal("second timeout/refenced claim reset retry count")
	}
}
