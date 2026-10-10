package goal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each process has its own two-connection pool. A separate observer connection
// controls real PostgreSQL contention, rather than relying on goroutine timing.
func processPool(t *testing.T, f *fixture, label string) (*pgxpool.Pool, string) {
	t.Helper()
	cfg := f.pool.Config()
	cfg.MaxConns, cfg.MinConns = 2, 1
	name := fmt.Sprintf("goal_%s_%d", label, time.Now().UnixNano())
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	p, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p, name
}
func observer(t *testing.T, f *fixture) *pgx.Conn {
	t.Helper()
	p, err := pgx.ConnectConfig(f.ctx, f.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = p.Close(ctx)
	})
	return p
}
func awaitLock(t *testing.T, ctx context.Context, p *pgx.Conn, name string, unfinished ...<-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(unfinished) > 0 {
			select {
			case err := <-unfinished[0]:
				t.Fatal("operation completed before held boundary", err)
			default:
			}
		}
		var waiting bool
		if err := p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1 AND wait_event_type='Lock')`, name).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("expected lock boundary not reached", name)
		case <-ticker.C:
		}
	}
}
func holdWrite(t *testing.T, f *fixture, p *pgx.Conn, table, operation, condition string) func() {
	t.Helper()
	key := time.Now().UnixNano()
	name := pgx.Identifier{fmt.Sprintf("goal_boundary_%d", key)}.Sanitize()
	if _, err := p.Exec(f.ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $barrier$ BEGIN IF %s THEN PERFORM pg_advisory_xact_lock(%d); END IF; RETURN NEW; END; $barrier$; CREATE TRIGGER %s BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, name, condition, key, name, operation, pgx.Identifier{table}.Sanitize(), name)
	if _, err := p.Exec(f.ctx, query); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := p.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(func() {
		release()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := p.Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON %s; DROP FUNCTION %s()`, name, pgx.Identifier{table}.Sanitize(), name)); err != nil {
			t.Error(err)
		}
	})
	return release
}
func await[T any](t *testing.T, ctx context.Context, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatal("authority boundary exceeded deadline", ctx.Err())
		var zero T
		return zero
	}
}
func ownerContext(ctx context.Context, f *fixture, delivery string) context.Context {
	return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.owner, Channel: "web", DeliveryID: delivery})
}

type ownerDecision struct {
	receipt goal.Receipt
	err     error
}

func changeAuthority(ctx context.Context, manager *goal.Manager, f *fixture, kind string) ownerDecision {
	if kind == "goal_amend" {
		g := f.g
		g.Constraints = []string{"Prepare only until owner reviews"}
		r, e := manager.PutGoal(ownerContext(ctx, f, kind), goal.GoalRequest{Goal: g, ExpectedRevision: g.Revision})
		return ownerDecision{r, e}
	}
	m := f.m
	m.Status = kind
	r, e := manager.PutMandate(ownerContext(ctx, f, kind), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
	return ownerDecision{r, e}
}

type liveContext struct{ pool *pgxpool.Pool }

func (r liveContext) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	var c actionauth.ContextSnapshot
	err := r.pool.QueryRow(ctx, `SELECT r.run_id,r.current_state,r.session_key,s.user_id,t.task_id,r.task_revision,t.revision,t.constraints_revision,t.status,t.execution_policy,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key AND o.run_id=r.run_id WHERE r.run_id=$1`, id).Scan(&c.RunID, &c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.PinnedTaskRevision, &c.TaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.ExecutionPolicy, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return c, err
	}
	a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, c.TaskID, c.OwnerID, nil, at)
	if err != nil {
		return c, err
	}
	c.Authority = &a
	c.MandateID = a.MandateID
	c.AutonomyMode = 3
	m, g, err := actionauth.CheckAssignmentTx(ctx, tx, a, at)
	if err != nil {
		return c, err
	}
	c.AuthorityDeadline, err = actionauth.AuthorityDeadline(m, g, at)
	if err != nil {
		return c, err
	}
	c.DenyTools, c.DenyResources, err = actionauth.ReadGoalDenialsTx(ctx, tx, a.GoalID, a.OwnerID, at)
	return c, err
}
func (liveContext) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return actionauth.ApprovalSnapshot{}, errors.New("this fixture exercises owner mandates, not exact approvals")
}

type allowedAction struct {
	service   *actionauth.Service
	store     *actionauth.PostgresStore
	grant     actionauth.Grant
	effect    actionauth.EffectRecord
	call      *toolbrokerv1.ToolCall
	contract  *toolbrokerv1.ToolContract
	recipient *httptest.Server
	posts     *atomic.Int32
}

func newAllowedAction(t *testing.T, f *fixture, pool *pgxpool.Pool, loseACK bool) allowedAction {
	t.Helper()
	posts := &atomic.Int32{}
	recipient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Idempotency-Key") == "" || r.Header.Get("Butler-Action-Fingerprint") == "" {
			w.WriteHeader(400)
			return
		}
		posts.Add(1)
		if loseACK {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"butler_effect_receipt": map[string]string{"idempotency_key": r.Header.Get("Idempotency-Key"), "action_fingerprint": r.Header.Get("Butler-Action-Fingerprint"), "effect_state": "effect_confirmed", "artifact_id": "synthetic-recovery-receipt"}})
	}))
	t.Cleanup(recipient.Close)
	m := f.m
	m.ResourceScopes = []actionauth.ResourceScope{{Origin: recipient.URL, PathPrefix: "/test-service"}}
	r, err := f.manager.PutMandate(f.actor("recipient-scope"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
	if err != nil {
		t.Fatal(err)
	}
	f.m, err = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	taskRecord := f.create(t, "allowed-task")
	id, _ := f.run(t, taskRecord, "tool_pending")
	private := httptest.NewServer(actionauth.NewContextHandler(strings.Repeat("s", 40), liveContext{pool}))
	t.Cleanup(private.Close)
	reader, err := actionauth.NewHTTPContextReader(private.URL, strings.Repeat("s", 40))
	if err != nil {
		t.Fatal(err)
	}
	store := actionauth.NewPostgresStore(pool)
	c := budget.NewController(pool, budget.DefaultPolicy())
	c.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	store.SetBudgets(c)
	s := actionauth.NewService(store, reader, nil)
	call := &toolbrokerv1.ToolCall{RunId: id, ToolCallId: "recover-synthetic-service", ToolName: "http.request", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"method":"POST","url":%q}`, recipient.URL+"/test-service/recover?procedure=test")}
	contract := &toolbrokerv1.ToolContract{ToolName: "http.request", Status: "enabled", InputSchemaJson: `{"type":"object"}`, ContractRevision: "synthetic-procedure-v1", RuntimeTarget: "fixture:http", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: recipient.URL}}
	prepared, err := s.PrepareWithLogicalRef(f.ctx, call, contract, "trusted-recovery-slot")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RequiresApproval || prepared.Binding.GoalID != f.g.ID || prepared.Binding.MandateID != f.m.ID {
		t.Fatal("compatible initiative required blanket/per-action permission", prepared)
	}
	grant, err := s.Authorize(f.ctx, call, contract, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	effect, err := s.EffectForGrant(f.ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	return allowedAction{s, store, grant, effect, call, contract, recipient, posts}
}
func (a allowedAction) dispatch(t *testing.T, ctx context.Context, loseACK bool) actionauth.EffectRecord {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, "POST", a.grant.Binding.Resource, strings.NewReader("synthetic recovery"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Idempotency-Key", a.effect.IdempotencyKey)
	request.Header.Set("Butler-Action-Fingerprint", a.effect.Fingerprint)
	response, err := a.recipient.Client().Do(request)
	var result *toolbrokerv1.ToolResult
	if loseACK {
		if err == nil {
			response.Body.Close()
			t.Fatal("expected lost acknowledgment")
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
	r, err := a.service.Outcome(ctx, a.grant, result, loseACK)
	if err != nil {
		t.Fatal(err)
	}
	want := actionauth.StateConfirmed
	if loseACK {
		want = actionauth.StateUnknown
	}
	if r.State != want || len(r.EvidenceRefs) != 1 || a.posts.Load() != 1 {
		t.Fatal("recipient fact and durable journal differ", r, a.posts.Load())
	}
	return r
}

func TestOwnerAuthorityChangeWinsBeforeRealDispatchIntegration(t *testing.T) {
	for _, kind := range []string{"revoked", "paused", "goal_amend"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			worker, workerName := processPool(t, f, "claim")
			owner, ownerName := processPool(t, f, "owner")
			a := newAllowedAction(t, f, worker, false)
			o := observer(t, f)
			table := "action_mandates"
			if kind == "goal_amend" {
				table = "owner_goals"
			}
			release := holdWrite(t, f, o, table, "UPDATE", "NEW.revision > OLD.revision")
			ctx, cancel := context.WithTimeout(f.ctx, 6*time.Second)
			defer cancel()
			changed := make(chan ownerDecision, 1)
			go func() { changed <- changeAuthority(ctx, goal.New(owner, task.NewStore(owner), nil), f, kind) }()
			awaitLock(t, ctx, o, ownerName)
			claimed := make(chan error, 1)
			go func() { claimed <- a.store.ClaimGrant(ctx, a.grant.ID) }()
			awaitLock(t, ctx, o, workerName, claimed)
			release()
			if r := await(t, ctx, changed); r.err != nil || r.receipt.Propagation != "pending" {
				t.Fatal(r)
			}
			if err := await(t, ctx, claimed); err == nil {
				a.dispatch(t, ctx, false)
				t.Fatal("stale grant crossed dispatch boundary")
			}
			if a.posts.Load() != 0 {
				t.Fatal("revoked authority reached recipient")
			}
			stored, err := a.store.GetGrant(ctx, a.grant.ID)
			if err != nil || stored.State != "authorized" {
				t.Fatal("denial spent grant", stored, err)
			}
			var n int
			if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&n); err != nil || n != 0 {
				t.Fatal("denial spent budget", n, err)
			}
		})
	}
}

func TestDispatchCommitBeforeOwnerRevocationRetainsConfirmedOrUnknownEvidenceIntegration(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint("lost_ack=", lost), func(t *testing.T) {
			f := setup(t)
			worker, workerName := processPool(t, f, "claim_first")
			owner, ownerName := processPool(t, f, "owner_second")
			a := newAllowedAction(t, f, worker, lost)
			o := observer(t, f)
			release := holdWrite(t, f, o, "action_authorizations", "UPDATE", "NEW.state='inflight'")
			ctx, cancel := context.WithTimeout(f.ctx, 6*time.Second)
			defer cancel()
			claimed := make(chan error, 1)
			go func() { claimed <- a.store.ClaimGrant(ctx, a.grant.ID) }()
			awaitLock(t, ctx, o, workerName, claimed)
			changed := make(chan ownerDecision, 1)
			go func() { changed <- changeAuthority(ctx, goal.New(owner, task.NewStore(owner), nil), f, "revoked") }()
			awaitLock(t, ctx, o, ownerName)
			release()
			if err := await(t, ctx, claimed); err != nil {
				t.Fatal(err)
			}
			if r := await(t, ctx, changed); r.err != nil {
				t.Fatal(r.err)
			}
			record := a.dispatch(t, ctx, lost)
			if err := a.store.ClaimGrant(ctx, a.grant.ID); err == nil {
				t.Fatal("revocation replayed an already spent dispatch")
			}
			saved, err := a.store.GetEffect(ctx, record.ActionID)
			if err != nil || saved.State != record.State || len(saved.EvidenceRefs) != 1 {
				t.Fatal("authority change discarded immutable effect evidence", saved, err)
			}
		})
	}
}

func TestAtomicResultCommitSerializesBeforeOwnerRevocationIntegration(t *testing.T) {
	f := setup(t)
	record := f.create(t, "task")
	id, runCtx := f.run(t, record, "finalizing")
	worker, workerName := processPool(t, f, "save")
	owner, ownerName := processPool(t, f, "revoke")
	o := observer(t, f)
	release := holdWrite(t, f, o, "task_results", "INSERT", "TRUE")
	ctx, cancel := context.WithTimeout(runCtx, 6*time.Second)
	defer cancel()
	saved := make(chan error, 1)
	go func() {
		_, err := completion.New(worker).Save(ctx, completion.Request{RunID: id, SessionKey: f.key, TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, Content: "Saved synthetic diagnostic evidence", AnswerState: "final"})
		saved <- err
	}()
	awaitLock(t, ctx, o, workerName)
	changed := make(chan ownerDecision, 1)
	go func() { changed <- changeAuthority(ctx, goal.New(owner, task.NewStore(owner), nil), f, "revoked") }()
	awaitLock(t, ctx, o, ownerName)
	release()
	if err := await(t, ctx, saved); err != nil {
		t.Fatal(err)
	}
	if r := await(t, ctx, changed); r.err != nil {
		t.Fatal(r.err)
	}
	if err := f.manager.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"run_final_results", "delivery_outbox", "task_results"} {
		var n int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE run_id=$1`, id).Scan(&n); err != nil || n != 1 {
			t.Fatal("authority change erased committed result or delivery", table, n, err)
		}
	}
	result, err := completion.New(f.pool).GetByRun(f.ctx, id)
	if err != nil || result.Verdict != "unverified" {
		t.Fatal("model final became verified owner goal", result, err)
	}
}

func TestCurrentAdmissionCommitsBeforeOwnerAmendmentThenBecomesStaleIntegration(t *testing.T) {
	f := setup(t)
	worker, workerName := processPool(t, f, "admit")
	owner, ownerName := processPool(t, f, "amend")
	o := observer(t, f)
	release := holdWrite(t, f, o, "task_authority_bindings", "INSERT", "TRUE")
	ctx, cancel := context.WithTimeout(f.ctx, 6*time.Second)
	defer cancel()
	admitted := make(chan error, 1)
	var newID string
	go func() {
		record, err := goal.New(worker, task.NewStore(worker), nil).CreateOwnerTask(ownerContext(ctx, f, "contended-admission"), goal.TaskRequest{SessionKey: f.key, GoalID: f.g.ID, MandateID: f.m.ID, Purpose: f.g.Purpose, Goal: "Test service diagnostics", CompletionCriteria: []string{"owner checks"}, ObservationRef: "synthetic:observation", InitiativeReason: "maintain service"})
		newID = record.TaskID
		admitted <- err
	}()
	awaitLock(t, ctx, o, workerName)
	changed := make(chan ownerDecision, 1)
	go func() { changed <- changeAuthority(ctx, goal.New(owner, task.NewStore(owner), nil), f, "goal_amend") }()
	awaitLock(t, ctx, o, ownerName)
	release()
	if err := await(t, ctx, admitted); err != nil {
		t.Fatal(err)
	}
	if r := await(t, ctx, changed); r.err != nil {
		t.Fatal(r.err)
	}
	if _, _, _, err := f.manager.ReadTaskAuthority(ctx, newID); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("admission silently inherited later goal intent", err)
	}
	if err := f.manager.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	record, err := f.tasks.Get(f.ctx, newID)
	if err != nil || record.Status != "paused" {
		t.Fatal("amended goal branch not paused", record, err)
	}
}
