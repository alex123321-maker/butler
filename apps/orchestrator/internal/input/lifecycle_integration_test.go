package input

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/activity"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/ownership"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lifecycleProvider struct {
	calls         []transport.StartRunRequest
	active        map[string]bool
	cancellations int
}

func (*lifecycleProvider) Name() string { return "fixture" }
func (*lifecycleProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsToolCalls: true}, nil
}
func (p *lifecycleProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p.active[r.Context.RunID] {
		return nil, errors.New("run is already active")
	}
	if p.active == nil {
		p.active = make(map[string]bool)
	}
	p.active[r.Context.RunID] = true
	p.calls = append(p.calls, r)
	stream := make(chan transport.TransportEvent, 3)
	recovered := false
	for _, item := range r.InputItems {
		if strings.Contains(item.Content, "Recorded Butler action observation") {
			if item.Role != "user" {
				return nil, errors.New("recorded observation became a provider instruction")
			}
			recovered = true
		}
	}
	if recovered {
		delete(p.active, r.Context.RunID)
		stream <- transport.NewAssistantFinalEvent(r.Context.RunID, "fixture", transport.AssistantFinal{Content: "known result accepted", FinishReason: "completed"})
		stream <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, "fixture")
	} else {
		stream <- transport.TransportEvent{RunID: r.Context.RunID, EventType: transport.EventTypeToolCallRequested, ToolCall: &transport.ToolCallRequest{ToolCallRef: "stable-call", ToolName: "http.request", ArgsJSON: `{"method":"POST","url":"https://fixture.invalid/submit","body":"one"}`}}
	}
	close(stream)
	return stream, nil
}
func (*lifecycleProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected provider continuation")
}
func (*lifecycleProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("restart must reconstruct Butler context, not reuse provider ref")
}
func (p *lifecycleProvider) CancelRun(_ context.Context, r transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancellations++
	delete(p.active, r.RunID)
	return nil, nil
}

func TestDurableApprovalResumesInSameProcessIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, provider := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	waiting := f.assertReceipt(t, "waiting_for_user")
	if provider.active[waiting.RunID] || provider.cancellations != 1 {
		t.Fatal("durable suspension retained private provider context")
	}
	if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
		t.Fatal(err)
	}
	worker.process(f.ctx, f.claim(t))
	completed := f.assertReceipt(t, "completed")
	if completed.RunID != waiting.RunID || f.tools.calls != 1 || len(provider.calls) != 2 || provider.calls[1].Context.ProviderSessionRef != nil {
		t.Fatal("same-process approval failed to start fresh context with one dispatch")
	}
}

type lifecycleTools struct {
	pool      *pgxpool.Pool
	approvals *approvals.Service
	calls     int
	crash     context.CancelFunc
	unknown   bool
}

func (x *lifecycleTools) PrepareAction(ctx context.Context, c *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	var owner, taskID string
	var pinned, revision, constraints int64
	err := x.pool.QueryRow(ctx, `SELECT s.user_id,COALESCE(r.task_id,''),COALESCE(r.task_revision,0),COALESCE(t.revision,0),COALESCE(t.constraints_revision,0) FROM runs r JOIN sessions s USING(session_key) LEFT JOIN tasks t ON t.task_id=r.task_id WHERE r.run_id=$1`, c.GetRunId()).Scan(&owner, &taskID, &pinned, &revision, &constraints)
	if err != nil {
		return actionauth.PreparedAction{}, err
	}
	contract := &toolbrokerv1.ToolContract{ToolName: c.GetToolName(), Status: "active", ToolClass: "http", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{}`, RequiresApproval: true}
	b, _, err := actionauth.BuildBinding(actionauth.StableActionID(c.GetRunId(), c.GetToolCallId()), c, contract, actionauth.ContextSnapshot{RunID: c.GetRunId(), OwnerID: owner, TaskID: taskID, PinnedTaskRevision: pinned, TaskRevision: revision, ConstraintsRevision: constraints})
	return actionauth.PreparedAction{Binding: b, RequiresApproval: true, Summary: "POST exact body one to fixture.invalid/submit", ExpiresAt: time.Now().Add(time.Minute)}, err
}
func (x *lifecycleTools) AuthorizeAction(ctx context.Context, c *toolbrokerv1.ToolCall, actionID, approvalID, mandateID string) (string, error) {
	p, err := x.PrepareAction(ctx, c)
	if err != nil {
		return "", err
	}
	snapshot, err := x.approvals.VerifyActionApproval(ctx, actionauth.ApprovalRequest{ApprovalID: approvalID, Binding: p.Binding})
	if err != nil || snapshot.Status != approvals.StatusApproved || actionID != p.Binding.ActionID {
		return "", approvals.ErrActionInvalid
	}
	return "fixture-private-grant", nil
}
func (*lifecycleTools) ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	return nil, errors.New("authorization required")
}
func (x *lifecycleTools) ExecuteAuthorizedToolCall(ctx context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	if grant == "" {
		return nil, errors.New("missing grant")
	}
	x.calls++
	if x.crash != nil {
		x.crash()
		return nil, ctx.Err()
	}
	status := "completed"
	if x.unknown {
		status = "effect_unknown"
	}
	return &toolbrokerv1.ToolResult{RunId: c.GetRunId(), ToolCallId: c.GetToolCallId(), ToolName: c.GetToolName(), Status: status, ResultJson: `{"submitted":true}`}, nil
}

type faultCheckpoint struct {
	*Store
	stage string
	crash context.CancelFunc
}

type immediateDecisionDelivery struct {
	flow.NopDeliverySink
	service *approvals.Service
	err     error
}

func (d *immediateDecisionDelivery) DeliverApprovalRequest(ctx context.Context, r flow.ApprovalRequest) error {
	_, _, d.err = d.service.ResolveByApprovalID(approvals.WithActor(ctx, ownerActor()), approvals.ResolveByApprovalIDParams{ApprovalID: r.ApprovalID, Approved: true, ResolvedVia: "web", ResolvedBy: "owner", ActorType: "web", ActorID: "owner", ResolvedAt: time.Now().UTC()})
	return d.err
}

func TestApprovalDecisionBeforeNotificationReturnsIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	delivery := &immediateDecisionDelivery{service: f.approvals}
	worker, _ := f.worker(f.store, delivery)
	worker.process(f.ctx, f.claim(t))
	if delivery.err != nil {
		t.Fatal(delivery.err)
	}
	f.assertReceipt(t, "queued")
	restarted, _ := f.worker(f.store, nil)
	restarted.process(f.ctx, f.claim(t))
	f.assertReceipt(t, "completed")
	if f.tools.calls != 1 {
		t.Fatalf("early decision lost or repeated: %d", f.tools.calls)
	}
}

func TestApprovalExpiryWithoutOwnerReplyIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	a := f.decision(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE approvals SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE approval_id=$1`, a.ApprovalID); err != nil {
		t.Fatal(err)
	}
	if err := f.approvals.ExpireInvalidPending(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.decision(t).Status != approvals.StatusExpired {
		t.Fatal("expired pending decision invisible")
	}
	worker.process(f.ctx, f.claim(t))
	r := f.assertReceipt(t, "blocked")
	job, err := f.tasks.Get(f.ctx, r.TaskID)
	if err != nil || job.Status != "blocked" || f.tools.calls != 0 {
		t.Fatalf("expiry was technical failure or dispatch: %#v %d %v", job, f.tools.calls, err)
	}
	attempt, err := f.runs.GetRun(f.ctx, r.RunID)
	if err != nil || attempt.GetCurrentState().String() != "RUN_STATE_AWAITING_APPROVAL" {
		t.Fatalf("expiry failed run: %#v %v", attempt, err)
	}
}

func TestApprovalExpiresAfterDecisionBeforeDispatchIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
		t.Fatal(err)
	}
	a := f.decision(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE approvals SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE approval_id=$1`, a.ApprovalID); err != nil {
		t.Fatal(err)
	}
	restarted, _ := f.worker(f.store, nil)
	restarted.process(f.ctx, f.claim(t))
	r := f.assertReceipt(t, "blocked")
	job, err := f.tasks.Get(f.ctx, r.TaskID)
	if err != nil || job.Status != "blocked" || r.ErrorCode != "approval_invalidated" || f.tools.calls != 0 {
		t.Fatalf("expired approved permission dispatched or unexplained: %#v %#v %v", r, job, err)
	}
}

type crashAfterUserTranscript struct {
	*transcript.Store
	crash context.CancelFunc
}

func (s crashAfterUserTranscript) AppendMessage(ctx context.Context, m transcript.Message) (transcript.Message, error) {
	stored, err := s.Store.AppendMessage(ctx, m)
	if err == nil && m.Role == "user" {
		s.crash()
		return stored, ctx.Err()
	}
	return stored, err
}
func TestCrashAfterUserTranscriptRecoversPreparationOnceIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	crashCtx, crash := context.WithCancel(f.ctx)
	p := &lifecycleProvider{}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, crashAfterUserTranscript{Store: f.transcript, crash: crash}, p, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, ApprovalService: f.approvals, ActionAuthorizer: f.tools, Tools: f.tools}, nil)
	worker := NewService(f.store, f.sessions, executor, f.runs, nil)
	worker.process(crashCtx, f.claim(t))
	r := f.assertReceipt(t, "running")
	if len(p.calls) != 0 {
		t.Fatal("provider dispatched before preparation recovered")
	}
	restarted, _ := f.worker(f.store, nil)
	restarted.process(f.ctx, f.reclaimAfterCrash(t))
	waiting := f.assertReceipt(t, "waiting_for_user")
	if r.RunID != waiting.RunID {
		t.Fatal("preparation recovery created new attempt")
	}
	history, err := f.transcript.GetTranscript(f.ctx, f.key)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, m := range history.Messages {
		if m.RunID == r.RunID && m.Role == "user" {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("current input replayed into durable transcript %d times", users)
	}
}

func (s faultCheckpoint) SaveCheckpoint(ctx context.Context, id, stage, payload string) error {
	if stage == s.stage {
		s.crash()
		return ctx.Err()
	}
	return s.Store.SaveCheckpoint(ctx, id, stage, payload)
}

type lifecycleFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	store      *Store
	sessions   *session.PostgresRepository
	runs       *run.Service
	tasks      *task.Store
	transcript *transcript.Store
	approvals  *approvals.Service
	tools      *lifecycleTools
	key        string
	event      ingress.InputEvent
	receipt    *flow.ExecutionResult
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	f := &lifecycleFixture{ctx: ctx, pool: db.Pool(), key: fmt.Sprintf("integration:lifecycle:%d", time.Now().UnixNano())}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, f.key)
	})
	f.store = NewStore(f.pool)
	f.sessions = session.NewPostgresRepository(f.pool)
	f.runs = run.NewService(run.NewPostgresRepository(f.pool), nil)
	f.tasks = task.NewStore(f.pool)
	f.transcript = transcript.NewStore(f.pool)
	repo := approvals.NewPostgresRepository(f.pool)
	repo.SetCheckpointHooks(f.store)
	f.approvals = approvals.NewService(repo, nil)
	f.tools = &lifecycleTools{pool: f.pool, approvals: f.approvals}
	_, _, err = f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: f.key, UserID: "owner", Channel: "web", MetadataJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.tasks.Create(ctx, task.CreateRequest{SessionKey: f.key, OwnerID: "owner", Goal: "submit once", Constraints: []string{"body one"}, CompletionCriteria: []string{"owner verifies submission"}})
	if err != nil {
		t.Fatal(err)
	}
	f.event = ingress.InputEvent{EventID: "owner-input", IdempotencyKey: "owner-input", SessionKey: f.key, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: fmt.Sprintf(`{"text":"submit once","user_id":"owner","task_id":%q}`, job.TaskID), CreatedAt: time.Now().UTC()}
	worker, _ := f.worker(f.store, nil)
	f.receipt, err = worker.Execute(ctx, f.event)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *lifecycleFixture) worker(checkpoints flow.CheckpointStore, delivery flow.DeliverySink) (*Service, *lifecycleProvider) {
	p := &lifecycleProvider{}
	executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, p, flow.Config{ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: checkpoints, ApprovalService: f.approvals, ActionAuthorizer: f.tools, Tools: f.tools, Delivery: delivery}, nil)
	return NewService(f.store, f.sessions, executor, f.runs, nil), p
}
func (f *lifecycleFixture) claim(t *testing.T) Record {
	t.Helper()
	r, err := f.store.Claim(f.ctx, time.Minute)
	if err != nil || r.InputID != f.receipt.InputID {
		t.Fatalf("claim fixture: %#v %v", r, err)
	}
	return r
}
func (f *lifecycleFixture) decision(t *testing.T) approvals.Record {
	t.Helper()
	r, err := f.store.Get(f.ctx, f.receipt.InputID)
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := f.approvals.LoadContinuationByRun(f.ctx, r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func (f *lifecycleFixture) resolve(t *testing.T, approved bool, actor approvals.Actor) (approvals.Record, bool, error) {
	t.Helper()
	a := f.decision(t)
	return f.approvals.ResolveByToolCall(approvals.WithActor(f.ctx, actor), approvals.ResolveByToolCallParams{ToolCallID: a.ToolCallID, Approved: approved, ResolvedVia: actor.Channel, ResolvedBy: actor.OwnerID, ActorType: actor.Channel, ActorID: actor.OwnerID, ResolvedAt: time.Now().UTC()})
}
func ownerActor() approvals.Actor {
	return approvals.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "owner-decision"}
}
func (f *lifecycleFixture) assertReceipt(t *testing.T, status string) Record {
	t.Helper()
	r, err := f.store.Get(f.ctx, f.receipt.InputID)
	if err != nil || r.Status != status {
		t.Fatalf("receipt expected %s: %#v %v", status, r, err)
	}
	return r
}
func (f *lifecycleFixture) reclaimAfterCrash(t *testing.T) Record {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.receipt.InputID); err != nil {
		t.Fatal(err)
	}
	return f.claim(t)
}

func TestDurableApprovalRestartsSameRunAndActionIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	waiting := f.assertReceipt(t, "waiting_for_user")
	a := f.decision(t)
	var live bool
	if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM session_ownership WHERE session_key=$1 AND expires_at>clock_timestamp() AND lease_id<>'')`, f.key).Scan(&live); err != nil || live {
		t.Fatalf("waiting worker retained lease: %v %v", live, err)
	}
	if f.tools.calls != 0 {
		t.Fatal("unapproved dispatch")
	}
	if _, _, err := f.resolve(t, true, approvals.Actor{OwnerID: "intruder", Channel: "web", DeliveryID: "forged"}); !errors.Is(err, approvals.ErrUnauthorized) {
		t.Fatalf("wrong owner allowed: %v", err)
	}
	if _, _, err := f.resolve(t, true, approvals.Actor{OwnerID: "owner", Channel: "telegram", SessionKey: f.key, DeliveryID: "cross-channel"}); !errors.Is(err, approvals.ErrUnauthorized) {
		t.Fatalf("wrong channel allowed: %v", err)
	}
	if _, changed, err := f.resolve(t, true, ownerActor()); err != nil || !changed {
		t.Fatalf("decision: %v %v", changed, err)
	}
	if _, changed, err := f.resolve(t, true, ownerActor()); err != nil || changed {
		t.Fatalf("duplicate decision: %v %v", changed, err)
	}
	restarted, p := f.worker(f.store, nil)
	restarted.process(f.ctx, f.claim(t))
	finished := f.assertReceipt(t, "completed")
	if finished.RunID != waiting.RunID || f.tools.calls != 1 {
		t.Fatalf("same run/exactly one intended dispatch: %s %s %d", finished.RunID, waiting.RunID, f.tools.calls)
	}
	if len(p.calls) != 1 || p.calls[0].Context.ProviderSessionRef != nil {
		t.Fatal("recovery reused provider authority")
	}
	history, err := f.transcript.GetTranscript(f.ctx, f.key)
	if err != nil || len(history.ToolCalls) != 1 || history.ToolCalls[0].ToolCallID != a.ToolCallID {
		t.Fatalf("same action transcript: %#v %v", history, err)
	}
	job, err := f.tasks.Get(f.ctx, finished.TaskID)
	if err != nil || job.Status != "active" {
		t.Fatalf("model final completed owner task: %#v %v", job, err)
	}
}

func TestDurableApprovalDecisionRacesAndInvalidationIntegration(t *testing.T) {
	for _, mode := range []string{"deny", "racing", "cancel", "changed", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			f := newLifecycleFixture(t)
			worker, _ := f.worker(f.store, nil)
			worker.process(f.ctx, f.claim(t))
			r := f.assertReceipt(t, "waiting_for_user")
			a := f.decision(t)
			if mode == "racing" {
				var wg sync.WaitGroup
				changes := make(chan bool, 2)
				errs := make(chan error, 2)
				for _, yes := range []bool{true, false} {
					wg.Add(1)
					go func(yes bool) {
						defer wg.Done()
						actor := ownerActor()
						actor.DeliveryID = fmt.Sprint(yes)
						_, changed, err := f.resolve(t, yes, actor)
						changes <- changed
						errs <- err
					}(yes)
				}
				wg.Wait()
				close(changes)
				close(errs)
				count := 0
				for change := range changes {
					if change {
						count++
					}
				}
				for err := range errs {
					if err != nil {
						t.Fatal(err)
					}
				}
				if count != 1 {
					t.Fatalf("two competing decisions changed %d times", count)
				}
			} else {
				if mode == "cancel" {
					if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='cancelled',revision=revision+1 WHERE task_id=$1`, r.TaskID); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "changed" {
					if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1,revision=revision+1 WHERE task_id=$1`, r.TaskID); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "expiry" {
					if _, err := f.pool.Exec(f.ctx, `UPDATE approvals SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE approval_id=$1`, a.ApprovalID); err != nil {
						t.Fatal(err)
					}
				}
				resolved, _, err := f.resolve(t, mode != "deny", ownerActor())
				if err != nil {
					t.Fatal(err)
				}
				if mode != "deny" && resolved.Status != approvals.StatusExpired {
					t.Fatalf("stale permission not expired: %s", resolved.Status)
				}
			}
			restarted, _ := f.worker(f.store, nil)
			restarted.process(f.ctx, f.claim(t))
			latest := f.decision(t)
			expected := "blocked"
			if latest.Status == approvals.StatusApproved {
				expected = "completed"
			}
			f.assertReceipt(t, expected)
			if expected == "blocked" && f.tools.calls != 0 {
				t.Fatal("denied/invalid action executed")
			}
			if mode == "cancel" {
				job, _ := f.tasks.Get(f.ctx, r.TaskID)
				if job.Status != "cancelled" {
					t.Fatal("approval overwrote cancellation")
				}
			}
		})
	}
}

func TestDurableUnknownEffectAndKnownResultRecoveryIntegration(t *testing.T) {
	for _, mode := range []string{"dispatched-no-result", "known-result", "recorded-unknown"} {
		t.Run(mode, func(t *testing.T) {
			f := newLifecycleFixture(t)
			worker, _ := f.worker(f.store, nil)
			worker.process(f.ctx, f.claim(t))
			if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
				t.Fatal(err)
			}
			crashCtx, crash := context.WithCancel(f.ctx)
			var checkpoints flow.CheckpointStore = f.store
			if mode == "dispatched-no-result" {
				f.tools.crash = crash
			}
			if mode == "known-result" {
				checkpoints = faultCheckpoint{Store: f.store, stage: "tool_result_recorded", crash: crash}
			}
			if mode == "recorded-unknown" {
				f.tools.unknown = true
			}
			crashing, _ := f.worker(checkpoints, nil)
			crashing.process(crashCtx, f.claim(t))
			crash()
			f.tools.crash = nil
			if mode == "recorded-unknown" {
				f.assertReceipt(t, "blocked")
			} else {
				restarted, p := f.worker(f.store, nil)
				restarted.process(f.ctx, f.reclaimAfterCrash(t))
				expected := "blocked"
				if mode == "known-result" {
					expected = "completed"
					if len(p.calls) != 1 {
						t.Fatal("known result not fed to fresh provider")
					}
				}
				f.assertReceipt(t, expected)
			}
			if f.tools.calls != 1 {
				t.Fatalf("external effect replayed: %d", f.tools.calls)
			}
		})
	}
}

func TestStaleExecutorCannotPublishDurableAuthorityIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	claim := f.claim(t)
	worker.process(f.ctx, claim)
	r := f.assertReceipt(t, "waiting_for_user")
	leases := session.NewPostgresLeaseManager(f.pool)
	old, err := leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: r.RunID, OwnerID: "old", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1`, f.key); err != nil {
		t.Fatal(err)
	}
	fresh, err := leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: r.RunID, OwnerID: "fresh", TTL: time.Minute})
	if err != nil || fresh.FenceEpoch <= old.FenceEpoch {
		t.Fatalf("takeover fence: %#v %v", fresh, err)
	}
	ctx := ownership.WithToken(f.ctx, leases.Authority(), ownership.Token{SessionKey: f.key, RunID: r.RunID, LeaseID: old.LeaseID, FenceEpoch: old.FenceEpoch})
	if _, err = f.runs.RefreshExecutionLease(ctx, r.RunID, old.LeaseID); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale run write: %v", err)
	}
	job, _ := f.tasks.Get(f.ctx, r.TaskID)
	if _, err = f.tasks.SaveWorking(ctx, job.TaskID, job.WorkingRevision, task.WorkingState{Plan: []string{"stale"}}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale task write: %v", err)
	}
	if _, err = artifacts.NewPostgresRepository(f.pool).CreateArtifact(ctx, artifacts.CreateParams{ArtifactID: "stale-artifact", RunID: r.RunID, SessionKey: f.key}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale artifact: %v", err)
	}
	if _, err = activity.NewPostgresRepository(f.pool).CreateActivity(ctx, activity.CreateParams{RunID: r.RunID, SessionKey: f.key}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale activity: %v", err)
	}
	if _, err = deliveryevents.NewPostgresRepository(f.pool).CreateEvent(ctx, deliveryevents.CreateParams{RunID: r.RunID, SessionKey: f.key}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale delivery record: %v", err)
	}
	if _, err = working.NewStore(f.pool).Save(ctx, working.Snapshot{RunID: r.RunID, SessionKey: f.key}); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale working state: %v", err)
	}
	var continuation struct {
		Binding actionauth.ActionBinding `json:"binding"`
	}
	_, raw, _ := f.approvals.LoadContinuationByRun(f.ctx, r.RunID)
	_ = json.Unmarshal([]byte(raw), &continuation)
	if continuation.Binding.ActionID == "" {
		t.Fatal("missing exact binding")
	}
}

func TestApprovalCannotAuthorizeChangedSavedArgumentsIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
		t.Fatal(err)
	}
	a := f.decision(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE approval_actions SET continuation_json=jsonb_set(continuation_json,'{tool_call,ArgsJSON}',$2::jsonb) WHERE approval_id=$1`, a.ApprovalID, `"{\"method\":\"POST\",\"url\":\"https://fixture.invalid/submit\",\"body\":\"changed\"}"`); err != nil {
		t.Fatal(err)
	}
	restarted, _ := f.worker(f.store, nil)
	restarted.process(f.ctx, f.claim(t))
	f.assertReceipt(t, "blocked")
	if f.tools.calls != 0 {
		t.Fatal("changed saved arguments reused owner permission")
	}
}

func TestTelegramDecisionIsBoundToSavedChatIntegration(t *testing.T) {
	f := newLifecycleFixture(t)
	worker, _ := f.worker(f.store, nil)
	worker.process(f.ctx, f.claim(t))
	a := f.decision(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE approval_actions SET channel='telegram' WHERE approval_id=$1`, a.ApprovalID); err != nil {
		t.Fatal(err)
	}
	wrong := approvals.Actor{OwnerID: "owner", Channel: "telegram", SessionKey: "telegram:chat:other", DeliveryID: "callback-1"}
	if _, _, err := f.resolve(t, true, wrong); !errors.Is(err, approvals.ErrUnauthorized) {
		t.Fatalf("other Telegram chat authorized: %v", err)
	}
	wrong.SessionKey = f.key
	if _, changed, err := f.resolve(t, true, wrong); err != nil || !changed {
		t.Fatalf("saved Telegram chat proof denied: %v %v", changed, err)
	}
}
