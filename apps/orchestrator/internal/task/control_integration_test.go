package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type controlsFixture struct {
	ctx            context.Context
	pool           *pgxpool.Pool
	tasks          *task.Store
	controls       *task.Controller
	leases         *session.PostgresLeaseManager
	session, owner string
	root           task.Record
}

func newControlsFixture(t *testing.T) *controlsFixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := "controls:" + t.Name() + ":" + time.Now().Format("150405.000000000")
	_, err = db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'controls-owner','web')`, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key) })
	tasks := task.NewStore(db.Pool())
	leases := session.NewPostgresLeaseManager(db.Pool())
	controls := task.NewController(tasks, leases.Authority(), leases, run.NewPostgresRepository(db.Pool()), approval.NewPostgresRepository(db.Pool()), durableinput.NewStore(db.Pool()), transcript.NewStore(db.Pool()))
	root, err := tasks.Create(ctx, task.CreateRequest{SessionKey: key, OwnerID: "controls-owner", Goal: "Search, prepare and send", CompletionCriteria: []string{"delivered"}})
	if err != nil {
		t.Fatal(err)
	}
	return &controlsFixture{ctx, db.Pool(), tasks, controls, leases, key, "controls-owner", root}
}
func (f *controlsFixture) actor(delivery, channel string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.owner, Channel: channel, SessionKey: f.session, DeliveryID: delivery})
}
func (f *controlsFixture) startRun(t *testing.T, id, checkpoint string) (string, session.LeaseRecord) {
	t.Helper()
	runID := "run:" + id + ":" + f.session
	_, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,started_at,updated_at,status,current_state,autonomy_mode,model_provider,task_id,task_revision) SELECT $1,$2,$1,clock_timestamp(),clock_timestamp(),'tool_pending','tool_pending','mode_2','test',task_id,revision FROM tasks WHERE task_id=$3`, runID, f.session, id)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.session, RunID: runID, OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint) VALUES($1,$2,$1,$1,'web',$3,'{}',$4,$5,'waiting_for_user',$6)`, runID+":input", f.session, f.owner, id, runID, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	return runID, lease
}

func TestControlAmendStopsOldApprovalAndPreservesNewContext(t *testing.T) {
	f := newControlsFixture(t)
	runID, lease := f.startRun(t, f.root.TaskID, "awaiting_approval")
	_, err := f.pool.Exec(f.ctx, `INSERT INTO approvals(approval_id,run_id,session_key,tool_call_id,status,requested_via,tool_name,args_json) VALUES($1,$2,$3,'call','pending','web','send','{}')`, runID+":approval", runID, f.session)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO approval_actions(approval_id,action_id,action_hash,task_id,task_revision,constraints_revision,owner_id,channel,binding_json,continuation_version,continuation_json) VALUES($1,$1,'hash',$2,1,1,$3,'web','{}',1,'{"version":1}')`, runID+":approval", f.root.TaskID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.tasks.SaveWorking(f.ctx, f.root.TaskID, f.root.WorkingRevision, task.WorkingState{Plan: []string{"send"}, PendingActions: []string{"send"}, Scratch: map[string]any{"draft": "saved"}})
	if err != nil {
		t.Fatal(err)
	}
	req := task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: task.ControlAmend, Instruction: "стоп, только подготовь письмо", ExecutionPolicy: task.PolicyPrepareOnly}
	result, err := f.controls.Apply(f.actor("amend", "web"), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Revision != 2 || result.Task.ConstraintsRevision != 2 || result.Task.ExecutionPolicy != task.PolicyPrepareOnly || result.ContinuationInputID == "" || len(result.Task.Working.PendingActions) != 0 {
		t.Fatalf("amendment lost: %+v", result)
	}
	if !strings.Contains(strings.Join(result.Task.Constraints, " "), req.Instruction) {
		t.Fatal("full instruction missing from protected constraints")
	}
	var approvalStatus, oldInput, runState, content string
	if err = f.pool.QueryRow(f.ctx, `SELECT a.status,i.status,r.current_state FROM approvals a JOIN runs r USING(run_id) JOIN accepted_inputs i ON i.run_id=r.run_id WHERE a.approval_id=$1`, runID+":approval").Scan(&approvalStatus, &oldInput, &runState); err != nil {
		t.Fatal(err)
	}
	if approvalStatus != "expired" || oldInput != "cancelled" || runState != "cancelled" {
		t.Fatalf("old authority: %s/%s/%s", approvalStatus, oldInput, runState)
	}
	if _, err = f.leases.GetLease(f.ctx, lease.LeaseID); !errors.Is(err, session.ErrLeaseNotFound) {
		t.Fatal("old lease valid", err)
	}
	stale := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: f.session, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if _, release, err := ownership.Begin(stale); !errors.Is(err, ownership.ErrStale) {
		if release != nil {
			release()
		}
		t.Fatal("late executor valid", err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT content FROM messages WHERE message_id=$1`, "control:"+result.ControlID).Scan(&content); err != nil || content != req.Instruction {
		t.Fatal("control history", err, content)
	}
	var oldWorking []byte
	if err = f.pool.QueryRow(f.ctx, `SELECT previous_working_json FROM task_control_targets WHERE control_id=$1`, result.ControlID).Scan(&oldWorking); err != nil || !strings.Contains(string(oldWorking), "saved") {
		t.Fatal("working history erased", err)
	}
	replay, err := f.controls.Apply(f.actor("amend", "web"), req)
	if err != nil || replay.ControlID != result.ControlID {
		t.Fatal("duplicate control", err)
	}
	req.Instruction = "different payload"
	if _, err = f.controls.Apply(f.actor("amend", "web"), req); !errors.Is(err, task.ErrConflict) {
		t.Fatal("conflicting delivery accepted", err)
	}
	history, err := f.controls.History(f.ctx, f.root.TaskID)
	if err != nil || len(history) != 1 {
		t.Fatal("history", err)
	}
}

func TestControlStopPropagatesChildrenAndRetainsExternalObservations(t *testing.T) {
	f := newControlsFixture(t)
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.session, OwnerID: f.owner, ParentTaskID: f.root.TaskID, Goal: "send child", CompletionCriteria: []string{"sent"}})
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := f.startRun(t, child.TaskID, "tool_dispatching")
	_, err = f.pool.Exec(f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result,error) VALUES($1,$2,'send','{}','effect_unknown','test',clock_timestamp(),'{"remote_receipt":"already-committed"}','{}')`, runID+":call", runID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.controls.Apply(f.actor("stop", "web"), task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlStop})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AffectedTaskIDs) != 2 || result.Task.Status != "cancelled" || result.EffectBoundary != "dispatch_may_have_committed_reconciliation_required" {
		t.Fatalf("stop: %+v", result)
	}
	child, err = f.tasks.Get(f.ctx, child.TaskID)
	if err != nil || child.Status != "cancelled" {
		t.Fatal("child not cancelled", err)
	}
	var observation []byte
	if err = f.pool.QueryRow(f.ctx, `SELECT result FROM tool_calls WHERE run_id=$1`, runID).Scan(&observation); err != nil || !strings.Contains(string(observation), "already-committed") {
		t.Fatal("external evidence erased", err)
	}
}

func TestControlTwoChannelsCASAndOwnerProof(t *testing.T) {
	f := newControlsFixture(t)
	if _, err := f.controls.Apply(f.ctx, task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlStop}); !errors.Is(err, task.ErrUnauthorized) {
		t.Fatal("missing owner proof accepted", err)
	}
	bad := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "bad"})
	if _, err := f.controls.Apply(bad, task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlStop}); !errors.Is(err, task.ErrUnauthorized) {
		t.Fatal("wrong owner accepted", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, channel := range []string{"web", "telegram"} {
		wg.Add(1)
		go func(ch string) {
			defer wg.Done()
			_, err := f.controls.Apply(f.actor("cas-"+ch, ch), task.ControlRequest{TaskID: f.root.TaskID, ExpectedRevision: 1, Kind: task.ControlAmend, Instruction: "prepare only", ExecutionPolicy: task.PolicyPrepareOnly})
			results <- err
		}(channel)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, task.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("CAS %d/%d", successes, conflicts)
	}
	current, err := f.tasks.Get(f.ctx, f.root.TaskID)
	if err != nil || current.Revision != 2 {
		t.Fatal(err)
	}
	newChild, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.session, OwnerID: f.owner, ParentTaskID: current.TaskID, Goal: "new child", CompletionCriteria: []string{"draft"}})
	if err != nil || newChild.ExecutionPolicy != task.PolicyPrepareOnly || len(newChild.Constraints) == 0 {
		t.Fatal("new branch lost restrictions", err)
	}
	budgetRoot, err := f.tasks.BudgetRoot(f.ctx, newChild.TaskID)
	if err != nil || budgetRoot.TaskID != f.root.TaskID || budgetRoot.OwnerID != f.owner {
		t.Fatal("budget identity changed across branches", err)
	}
	if _, err = f.controls.ResolveTarget(f.actor("target", "web"), ""); err != nil {
		t.Fatal("unique root target", err)
	}
	_, err = f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.session, OwnerID: f.owner, Goal: "another root", CompletionCriteria: []string{"done"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.controls.ResolveTarget(f.actor("ambiguous", "web"), ""); !errors.Is(err, task.ErrConflict) {
		t.Fatal("ambiguous target guessed", err)
	}
	data, _ := json.Marshal(current)
	if !strings.Contains(string(data), "prepare_only") {
		t.Fatal("policy absent")
	}
}

func TestControlRollbackRefusesToDiscardOwnerAuthority(t *testing.T) {
	f := newControlsFixture(t)
	result, err := f.controls.Apply(f.actor("guard-rollback", "web"), task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlAmend, Instruction: "prepare only", ExecutionPolicy: task.PolicyPrepareOnly})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "036_task_owner_controls.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(f.ctx, string(sql))
	_ = tx.Rollback(f.ctx)
	if err == nil || !strings.Contains(err.Error(), "discard authority") {
		t.Fatal("rollback discarded live control history", err)
	}
	saved, err := f.tasks.Get(f.ctx, result.Task.TaskID)
	if err != nil || saved.ExecutionPolicy != task.PolicyPrepareOnly || saved.Revision != 2 {
		t.Fatal("rollback changed authority", err)
	}
}

func TestControlLockOrderAllowsPriorFencedTranscriptFromOtherProcess(t *testing.T) {
	f := newControlsFixture(t)
	runID, lease := f.startRun(t, f.root.TaskID, "model_started")
	tokenCtx := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: f.session, RunID: runID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	guarded, release, err := ownership.Begin(tokenCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	other, err := postgresstore.Open(f.ctx, postgresstore.Config{URL: os.Getenv("BUTLER_TEST_POSTGRES_URL"), MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	otherLease := session.NewPostgresLeaseManager(other.Pool())
	controller := task.NewController(task.NewStore(other.Pool()), otherLease.Authority(), otherLease, run.NewPostgresRepository(other.Pool()), approval.NewPostgresRepository(other.Pool()), durableinput.NewStore(other.Pool()), transcript.NewStore(other.Pool()))
	done := make(chan error, 1)
	go func() {
		_, err := controller.Apply(f.actor("other-process-stop", "web"), task.ControlRequest{TaskID: f.root.TaskID, Kind: task.ControlStop})
		done <- err
	}()
	select {
	case err = <-done:
		t.Fatal("control passed a live shared ownership guard", err)
	case <-time.After(100 * time.Millisecond):
	}
	// The transcript trigger needs the session admission row. An admission-first
	// controller would deadlock here against the existing ownership guard.
	writeCtx, cancel := context.WithTimeout(guarded, 2*time.Second)
	_, err = transcript.NewStore(f.pool).AppendMessage(writeCtx, transcript.Message{SessionKey: f.session, RunID: runID, Role: "assistant", Content: "prior valid partial observation"})
	cancel()
	if err != nil {
		t.Fatal("cross-process lock order blocked a prior authority write", err)
	}
	release()
	release = nil
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("control did not commit after guard release")
	}
}
