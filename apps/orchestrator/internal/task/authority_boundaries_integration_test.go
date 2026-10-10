package task_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Independent pools/authorities model separate controller, Broker and executor
// processes. Every business pool has exactly two connections. A separate pgx
// connection observes PostgreSQL locks and controls deterministic test barriers.
func boundaryPool(t *testing.T, f *controlsFixture, prefix string) (*pgxpool.Pool, string) {
	t.Helper()
	config := f.pool.Config()
	config.MaxConns, config.MinConns = 2, 1
	name := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	config.ConnConfig.RuntimeParams["application_name"] = name
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, name
}

func boundaryController(pool *pgxpool.Pool) *task.Controller {
	leases := session.NewPostgresLeaseManager(pool)
	return task.NewController(task.NewStore(pool), leases.Authority(), leases, run.NewPostgresRepository(pool), approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool))
}

func boundaryObserver(t *testing.T, f *controlsFixture) *pgx.Conn {
	t.Helper()
	connection, err := pgx.ConnectConfig(f.ctx, f.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = connection.Close(ctx)
	})
	return connection
}

// Wait for actual lock contention, not a timing assumption that a goroutine has
// already reached a boundary. Advisory waits and ownership waits are both visible.
func waitBoundaryLock(t *testing.T, f *controlsFixture, observer *pgx.Conn, application string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1 AND wait_event_type='Lock')`, application).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("process did not reach the expected PostgreSQL lock boundary", application)
		case <-ticker.C:
		}
	}
}

// The trigger pauses the real transaction after it holds its authority locks,
// before its irreversible commit. It never changes production authorization.
func holdBoundaryUpdate(t *testing.T, f *controlsFixture, observer *pgx.Conn, table, condition string) func() {
	t.Helper()
	key := time.Now().UnixNano()
	name := pgx.Identifier{fmt.Sprintf("owner_boundary_%d", key)}.Sanitize()
	if _, err := observer.Exec(f.ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $boundary$ BEGIN IF %s THEN PERFORM pg_advisory_xact_lock(%d); END IF; RETURN NEW; END; $boundary$; CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, name, condition, key, name, pgx.Identifier{table}.Sanitize(), name)
	if _, err := observer.Exec(f.ctx, sql); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := observer.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
				t.Error("release test boundary", err)
			}
		})
	}
	t.Cleanup(func() {
		release()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := observer.Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON %s; DROP FUNCTION %s()`, name, pgx.Identifier{table}.Sanitize(), name))
		if err != nil {
			t.Error("remove test boundary", err)
		}
	})
	return release
}

type boundaryContextReader struct {
	pool      *pgxpool.Pool
	approvals *approval.Service
}

func (r boundaryContextReader) GetActionContext(ctx context.Context, runID string) (actionauth.ContextSnapshot, error) {
	var c actionauth.ContextSnapshot
	err := r.pool.QueryRow(ctx, `SELECT r.run_id,r.current_state,r.session_key,s.user_id,COALESCE(r.task_id,''),r.task_revision,t.revision,t.constraints_revision,t.status,t.execution_policy,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key AND o.run_id=r.run_id WHERE r.run_id=$1`, runID).Scan(&c.RunID, &c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.PinnedTaskRevision, &c.TaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.ExecutionPolicy, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	c.AutonomyMode = 3
	return c, err
}
func (r boundaryContextReader) VerifyActionApproval(ctx context.Context, q actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return r.approvals.VerifyActionApproval(ctx, q)
}

type boundaryAction struct {
	service  *actionauth.Service
	store    *actionauth.PostgresStore
	grant    actionauth.Grant
	call     *toolbrokerv1.ToolCall
	contract *toolbrokerv1.ToolContract
	effect   actionauth.EffectRecord
	receiver *httptest.Server
	posts    *atomic.Int32
}

func newBoundaryAction(t *testing.T, f *controlsFixture, pool *pgxpool.Pool, runID string, lease session.LeaseRecord, loseACK bool) boundaryAction {
	t.Helper()
	posts := &atomic.Int32{}
	var recipientMu sync.Mutex
	accepted := map[string]string{}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, fingerprint := r.Header.Get("Idempotency-Key"), r.Header.Get("Butler-Action-Fingerprint")
		if r.Method != http.MethodPost || key == "" || fingerprint == "" {
			w.WriteHeader(400)
			return
		}
		recipientMu.Lock()
		previous, exists := accepted[key]
		if exists && previous != fingerprint {
			recipientMu.Unlock()
			w.WriteHeader(409)
			return
		}
		if !exists {
			accepted[key] = fingerprint
			posts.Add(1)
		}
		recipientMu.Unlock()
		if loseACK {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"butler_effect_receipt": map[string]string{"idempotency_key": r.Header.Get("Idempotency-Key"), "action_fingerprint": r.Header.Get("Butler-Action-Fingerprint"), "effect_state": string(actionauth.StateConfirmed), "artifact_id": "recipient-created-item"}})
	}))
	t.Cleanup(receiver.Close)
	inputs := durableinput.NewStore(pool)
	approvalRepo := approval.NewPostgresRepository(pool)
	approvalRepo.SetCheckpointHooks(inputs)
	approvalService := approval.NewService(approvalRepo, nil)
	token := "private-boundary-service-token-at-least-32-bytes"
	authority := httptest.NewServer(actionauth.NewContextHandler(token, boundaryContextReader{pool, approvalService}))
	t.Cleanup(authority.Close)
	reader, err := actionauth.NewHTTPContextReader(authority.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	store := actionauth.NewPostgresStore(pool)
	service := actionauth.NewService(store, reader, nil)
	call := &toolbrokerv1.ToolCall{RunId: runID, ToolCallId: "send-once", ToolName: "http.request", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"method":"POST","url":%q}`, receiver.URL+"/send")}
	contract := &toolbrokerv1.ToolContract{ToolName: call.ToolName, Status: "enabled", InputSchemaJson: `{"type":"object"}`, ContractRevision: "owner-boundary-receipt-v1", RuntimeTarget: "fixture:http", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: receiver.URL}}
	prepared, err := service.PrepareWithLogicalRef(f.ctx, call, contract, "trusted-send-slot")
	if err != nil {
		t.Fatal(err)
	}
	claimID := "boundary-durable-input-claim"
	if _, err = pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='running',claim_id=$2,claim_expires_at=clock_timestamp()+INTERVAL '1 minute' WHERE input_id=$1`, runID+":input", claimID); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	executionCtx := ownership.WithToken(f.ctx, leases.Authority(), ownership.Token{RunID: runID, SessionKey: f.session, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	executionCtx = durableinput.WithClaim(executionCtx, durableinput.Record{InputID: runID + ":input", ClaimID: claimID})
	pending, err := approvalService.CreatePendingApproval(executionCtx, approval.CreatePendingParams{RunID: runID, SessionKey: f.session, ToolCallID: call.ToolCallId, RequestedVia: "web", ToolName: call.ToolName, ArgsJSON: call.ArgsJson, PayloadJSON: "{}", ActionBinding: prepared.Binding, ContinuationJSON: `{"version":1}`, RequestedAt: time.Now(), ExpiresAt: &prepared.ExpiresAt})
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := approval.ActorFromContext(f.actor("approve-exact-boundary", "web"))
	if resolved, changed, err := approvalRepo.ResolveDurable(f.ctx, pending, true, actor, "approve exact fixture send"); err != nil || !changed || resolved.Status != "approved" {
		t.Fatal("durable exact approval failed", changed, err)
	}
	grant, err := service.Authorize(f.ctx, call, contract, prepared.Binding.ActionID, pending.ApprovalID, "")
	if err != nil {
		t.Fatal(err)
	}
	effect, err := service.EffectForGrant(f.ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	// The real tool handler persists this checkpoint before calling Broker.
	if _, err = pool.Exec(f.ctx, `UPDATE accepted_inputs SET checkpoint='tool_dispatching' WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	return boundaryAction{service, store, grant, call, contract, effect, receiver, posts}
}

// Dispatch is deliberately conditional on a successful actual ClaimGrant. It
// contacts a real HTTP recipient; no model verdict supplies outcome evidence.
func (a boundaryAction) dispatch(t *testing.T, ctx context.Context, loseACK bool) actionauth.EffectRecord {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.receiver.URL+"/send", strings.NewReader("fixture-send"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", a.effect.IdempotencyKey)
	request.Header.Set("Butler-Action-Fingerprint", a.effect.Fingerprint)
	response, err := a.receiver.Client().Do(request)
	var result *toolbrokerv1.ToolResult
	if loseACK {
		if err == nil {
			_ = response.Body.Close()
			t.Fatal("recipient did not lose acknowledgement")
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "body": string(body), "truncated": false})
		result = &toolbrokerv1.ToolResult{RunId: a.call.RunId, ToolCallId: a.call.ToolCallId, ToolName: a.call.ToolName, Status: "completed", ResultJson: string(encoded)}
	}
	record, err := a.service.Outcome(ctx, a.grant, result, loseACK)
	if err != nil {
		t.Fatal(err)
	}
	want := actionauth.StateConfirmed
	if loseACK {
		want = actionauth.StateUnknown
	}
	if record.State != want || a.posts.Load() != 1 || len(record.EvidenceRefs) != 1 {
		t.Fatalf("recipient/journal disagreed: state=%s posts=%d evidence=%v", record.State, a.posts.Load(), record.EvidenceRefs)
	}
	return record
}

func boundaryControlRequest(f *controlsFixture, kind string) task.ControlRequest {
	q := task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: kind}
	if kind == task.ControlAmend {
		q.Instruction, q.ExecutionPolicy = "стоп, пока только подготовь", task.PolicyPrepareOnly
	}
	return q
}

func awaitBoundary[T any](t *testing.T, ctx context.Context, channel <-chan T) T {
	t.Helper()
	select {
	case result := <-channel:
		return result
	case <-ctx.Done():
		t.Fatal("boundary operation exceeded deadline", ctx.Err())
		var zero T
		return zero
	}
}

type boundaryControlResult struct {
	receipt task.ControlResult
	err     error
}

func TestOwnerControlWinsBeforeRealDispatchClaimIntegration(t *testing.T) {
	for _, kind := range []string{task.ControlStop, task.ControlAmend} {
		t.Run(kind, func(t *testing.T) {
			f := newControlsFixture(t)
			runID, lease := f.startRun(t, f.root.TaskID, "tool_dispatching")
			workerPool, workerName := boundaryPool(t, f, "claim_after_owner")
			controllerPool, controllerName := boundaryPool(t, f, "owner_before_claim")
			a := newBoundaryAction(t, f, workerPool, runID, lease, false)
			observer := boundaryObserver(t, f)
			release := holdBoundaryUpdate(t, f, observer, "session_ownership", fmt.Sprintf("NEW.run_id=%s AND NEW.lease_id=''", sqlBoundaryLiteral(runID)))
			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			controlDone := make(chan boundaryControlResult, 1)
			go func() {
				r, err := boundaryController(controllerPool).Apply(approval.WithActor(ctx, approval.Actor{OwnerID: f.owner, Channel: "web", SessionKey: f.session, DeliveryID: "owner-first"}), boundaryControlRequest(f, kind))
				controlDone <- boundaryControlResult{r, err}
			}()
			waitBoundaryLock(t, f, observer, controllerName)
			claimDone := make(chan error, 1)
			go func() { claimDone <- a.store.ClaimGrant(ctx, a.grant.ID) }()
			waitBoundaryLock(t, f, observer, workerName)
			release()
			if result := awaitBoundary(t, ctx, controlDone); result.err != nil || result.receipt.Task.Revision != 2 {
				t.Fatal("owner control failed", result.err)
			}
			if err := awaitBoundary(t, ctx, claimDone); err == nil {
				// A real sender would only contact the recipient after successful claim.
				_ = a.dispatch(t, ctx, false)
				t.Fatal("obsolete grant crossed the irreversible boundary")
			}
			record, err := a.store.GetEffect(f.ctx, a.effect.ActionID)
			if err != nil || record.State != actionauth.StateAllowed || len(record.EvidenceRefs) != 0 || a.posts.Load() != 0 {
				t.Fatal("stopped proposal produced an external effect", err)
			}
			var dispatches int
			if err = workerPool.QueryRow(f.ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, a.grant.ID).Scan(&dispatches); err != nil || dispatches != 0 {
				t.Fatal("cancelled claim recorded dispatch", err)
			}
			if _, err = a.service.Verify(f.ctx, a.call, a.contract, a.grant.ID); err == nil {
				t.Fatal("obsolete approved proposal still verifies")
			}
			if _, err = a.service.Authorize(f.ctx, a.call, a.contract, a.grant.Binding.ActionID, a.grant.ApprovalID, ""); err == nil {
				t.Fatal("old approved proposal refreshed after amendment")
			}
		})
	}
}

func sqlBoundaryLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func TestOwnerTreeStopRevokesActualGrandchildGrantIntegration(t *testing.T) {
	f := newControlsFixture(t)
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.session, OwnerID: f.owner, ParentTaskID: f.root.TaskID, Goal: "prepare recipient action", CompletionCriteria: []string{"reviewed"}})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.session, OwnerID: f.owner, ParentTaskID: child.TaskID, Goal: "send recipient action", CompletionCriteria: []string{"receipt"}})
	if err != nil {
		t.Fatal(err)
	}
	runID, lease := f.startRun(t, grandchild.TaskID, "tool_dispatching")
	workerPool, _ := boundaryPool(t, f, "grandchild_authority")
	a := newBoundaryAction(t, f, workerPool, runID, lease, false)
	control, err := f.controls.Apply(f.actor("stop-whole-tree", "web"), boundaryControlRequest(f, task.ControlStop))
	if err != nil || len(control.AffectedTaskIDs) != 3 {
		t.Fatal("tree stop did not cover the full hierarchy", err)
	}
	for _, id := range []string{f.root.TaskID, child.TaskID, grandchild.TaskID} {
		current, err := f.tasks.Get(f.ctx, id)
		if err != nil || current.Status != "cancelled" || current.Revision != 2 {
			t.Fatal("descendant authority survived stop", id, err)
		}
	}
	if err = a.store.ClaimGrant(f.ctx, a.grant.ID); err == nil {
		t.Fatal("old grandchild grant could dispatch after parent stop")
	}
	if _, err = a.service.Verify(f.ctx, a.call, a.contract, a.grant.ID); err == nil || a.posts.Load() != 0 {
		t.Fatal("approved descendant action remained authorized", err)
	}
}

func TestRealDispatchClaimWinsBeforeOwnerControlRetainsOutcomeIntegration(t *testing.T) {
	for _, kind := range []string{task.ControlStop, task.ControlAmend} {
		for _, loseACK := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lost_ack_%t", kind, loseACK), func(t *testing.T) {
				f := newControlsFixture(t)
				runID, lease := f.startRun(t, f.root.TaskID, "tool_dispatching")
				workerPool, workerName := boundaryPool(t, f, "claim_before_owner")
				controllerPool, controllerName := boundaryPool(t, f, "owner_after_claim")
				a := newBoundaryAction(t, f, workerPool, runID, lease, loseACK)
				observer := boundaryObserver(t, f)
				release := holdBoundaryUpdate(t, f, observer, "action_authorizations", fmt.Sprintf("NEW.authorization_id=%s AND NEW.state='inflight'", sqlBoundaryLiteral(a.grant.ID)))
				ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
				defer cancel()
				claimDone := make(chan error, 1)
				go func() { claimDone <- a.store.ClaimGrant(ctx, a.grant.ID) }()
				waitBoundaryLock(t, f, observer, workerName)
				controlDone := make(chan boundaryControlResult, 1)
				go func() {
					r, err := boundaryController(controllerPool).Apply(approval.WithActor(ctx, approval.Actor{OwnerID: f.owner, Channel: "web", SessionKey: f.session, DeliveryID: "claim-first"}), boundaryControlRequest(f, kind))
					controlDone <- boundaryControlResult{r, err}
				}()
				waitBoundaryLock(t, f, observer, controllerName)
				release()
				if err := awaitBoundary(t, ctx, claimDone); err != nil {
					t.Fatal("earlier dispatch claim failed", err)
				}
				control := awaitBoundary(t, ctx, controlDone)
				if control.err != nil || control.receipt.EffectBoundary != "dispatch_may_have_committed_reconciliation_required" {
					t.Fatal("owner receipt promised prevention after claim", control.err)
				}
				// The already claimed request may reach the recipient after the owner
				// commits stop. The Broker journal must retain its later observation.
				record := a.dispatch(t, f.ctx, loseACK)
				restarted := actionauth.NewPostgresStore(workerPool)
				saved, err := restarted.GetEffect(f.ctx, record.ActionID)
				if err != nil || saved.State != record.State || len(saved.EvidenceRefs) != 1 {
					t.Fatal("restart lost the external observation", err)
				}
				evidence, err := restarted.GetEffectEvidence(f.ctx, saved.EvidenceRefs[0])
				if err != nil || evidence.SourceRunID != runID || evidence.Verdict != record.State || evidence.ObservedAt.IsZero() {
					t.Fatal("observation lacks immutable provenance", err)
				}
				if _, err = workerPool.Exec(f.ctx, `UPDATE action_effect_evidence SET evidence='{}' WHERE evidence_id=$1`, evidence.EvidenceID); err == nil {
					t.Fatal("owner cancellation made evidence mutable")
				}
				if err = restarted.ClaimGrant(f.ctx, a.grant.ID); err == nil || a.posts.Load() != 1 {
					t.Fatal("claimed effect could replay after stop", err)
				}
			})
		}
	}
}

type boundaryEffectReader struct{ store *actionauth.PostgresStore }

func (r boundaryEffectReader) ReadRunEffects(ctx context.Context, id string) ([]completion.Effect, error) {
	items, err := r.store.GetRunEvidence(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]completion.Effect, 0, len(items))
	for _, item := range items {
		result = append(result, completion.Effect{ActionID: item.ActionID, EffectState: string(item.EffectState), EvidenceRefs: item.EvidenceRefs})
	}
	return result, nil
}

type boundaryFinalResult struct {
	result completion.FinalRecord
	err    error
}

type boundaryArtifactGate struct {
	real    *artifacts.PostgresRepository
	entered chan struct{}
	release <-chan struct{}
}

func (g boundaryArtifactGate) SaveFinalTx(ctx context.Context, tx pgx.Tx, q artifacts.CreateParams) error {
	if err := g.real.SaveFinalTx(ctx, tx, q); err != nil {
		return err
	}
	close(g.entered)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestOwnerStopAndAtomicResultCommitOrderIntegration(t *testing.T) {
	for _, first := range []string{"stop", "result"} {
		t.Run(first, func(t *testing.T) {
			f := newControlsFixture(t)
			runID, lease := f.startRun(t, f.root.TaskID, "tool_result_recorded")
			workerPool, workerName := boundaryPool(t, f, "result_boundary")
			controllerPool, controllerName := boundaryPool(t, f, "stop_result_boundary")
			a := newBoundaryAction(t, f, workerPool, runID, lease, false)
			if err := a.store.ClaimGrant(f.ctx, a.grant.ID); err != nil {
				t.Fatal(err)
			}
			external := a.dispatch(t, f.ctx, false)
			if _, err := workerPool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing',lease_id=$2 WHERE run_id=$1`, runID, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			coordinator := completion.New(workerPool)
			coordinator.Evidence = boundaryEffectReader{a.store}
			workerLeases := session.NewPostgresLeaseManager(workerPool)
			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			executionCtx := ownership.WithToken(ctx, workerLeases.Authority(), ownership.Token{RunID: runID, SessionKey: f.session, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			q := completion.Request{RunID: runID, SessionKey: f.session, TaskID: f.root.TaskID, TaskRevision: 1, ConstraintsRevision: 1, Content: "Recipient accepted one send; owner verification remains required", AnswerState: "final", PipelineEnabled: true}
			observer := boundaryObserver(t, f)
			controlDone := make(chan boundaryControlResult, 1)
			finalDone := make(chan boundaryFinalResult, 1)
			stop := func() {
				r, err := boundaryController(controllerPool).Apply(approval.WithActor(ctx, approval.Actor{OwnerID: f.owner, Channel: "web", SessionKey: f.session, DeliveryID: "stop-vs-result"}), boundaryControlRequest(f, task.ControlStop))
				controlDone <- boundaryControlResult{r, err}
			}
			save := func() {
				r, err := coordinator.Save(executionCtx, q)
				finalDone <- boundaryFinalResult{r, err}
			}
			if first == "stop" {
				release := holdBoundaryUpdate(t, f, observer, "session_ownership", fmt.Sprintf("NEW.run_id=%s AND NEW.lease_id=''", sqlBoundaryLiteral(runID)))
				go stop()
				waitBoundaryLock(t, f, observer, controllerName)
				go save()
				waitBoundaryLock(t, f, observer, workerName)
				release()
			} else {
				entered, finish := make(chan struct{}), make(chan struct{})
				var once sync.Once
				release := func() { once.Do(func() { close(finish) }) }
				t.Cleanup(release)
				coordinator.Artifacts = boundaryArtifactGate{artifacts.NewPostgresRepository(workerPool), entered, finish}
				go save()
				awaitBoundary(t, ctx, entered)
				go stop()
				waitBoundaryLock(t, f, observer, controllerName)
				release()
			}
			control := awaitBoundary(t, ctx, controlDone)
			final := awaitBoundary(t, ctx, finalDone)
			if control.err != nil || control.receipt.Task.Status != "cancelled" || control.receipt.Task.Revision != 2 {
				t.Fatal("stop failed", control.err)
			}
			want := 0
			if first == "result" {
				want = 1
				if final.err != nil || final.result.Verdict != "unverified" || len(final.result.Effects) != 1 || final.result.Effects[0].ActionID != external.ActionID || len(final.result.Effects[0].EvidenceRefs) != 1 {
					t.Fatal("earlier atomic result lost journal evidence", final.err)
				}
			} else if final.err == nil {
				t.Fatal("revoked executor saved a final result")
			}
			for _, table := range []string{"run_final_results", "artifacts", "task_results", "delivery_outbox"} {
				var count int
				if err := workerPool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE run_id=$1`, runID).Scan(&count); err != nil || count != want {
					t.Fatal("wrong atomic persistence boundary", table, count, err)
				}
			}
			var finalMessages, auxiliary, runCompleted int
			if err := workerPool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM messages WHERE run_id=$1 AND role='assistant'),(SELECT count(*) FROM completion_aux_jobs WHERE run_id=$1),(SELECT count(*) FROM runs WHERE run_id=$1 AND current_state='completed')`, runID).Scan(&finalMessages, &auxiliary, &runCompleted); err != nil || finalMessages != want || auxiliary != want*3 || runCompleted != want {
				t.Fatal("partial atomic final escaped", finalMessages, auxiliary, runCompleted, err)
			}
			retained, err := a.store.GetEffect(f.ctx, external.ActionID)
			if err != nil || retained.State != actionauth.StateConfirmed || len(retained.EvidenceRefs) != 1 || a.posts.Load() != 1 {
				t.Fatal("stop erased an actual recipient effect", err)
			}
			if first == "result" {
				read, err := completion.New(workerPool).GetByRun(f.ctx, runID)
				if err != nil || read.Content != q.Content || read.TaskRevision != 1 || read.ResultID != final.result.ResultID {
					t.Fatal("historical result lost after new task revision", err)
				}
				rows, err := coordinator.Outbox.ListByRun(f.ctx, runID)
				if err != nil || len(rows) != 1 || rows[0].Status != "result_ready" || !strings.Contains(rows[0].Content, q.Content) {
					t.Fatal("saved notification was rewritten by stop", err)
				}
				for _, statement := range []string{`UPDATE run_final_results SET content='rewritten' WHERE run_id=$1`, `UPDATE artifacts SET content_text='rewritten' WHERE run_id=$1`, `UPDATE messages SET content='rewritten' WHERE run_id=$1 AND role='assistant'`, `UPDATE delivery_outbox SET content='rewritten' WHERE run_id=$1`} {
					if _, err := workerPool.Exec(f.ctx, statement, runID); err == nil {
						t.Fatal("retained atomic component is mutable")
					}
				}
			}
			if _, err = coordinator.Save(executionCtx, q); err == nil {
				t.Fatal("old worker could write after owner stop")
			}
		})
	}
}
