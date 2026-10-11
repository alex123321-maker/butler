package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dispatchAdmissionFixture struct {
	f            *delegationGuardFixture
	service      *DelegationAdmission
	reader       *actionContextReader
	approvals    *approvals.Service
	journal      *actionauth.PostgresStore
	auth         *actionauth.Service
	owned        context.Context
	args         delegation.AdmissionArgs
	call         *toolbrokerv1.ToolCall
	prepared     actionauth.PreparedAction
	grant        actionauth.Grant
	envelope     delegation.AdmissionEnvelope
	manifestPath string
	inputClaim   durableinput.Record
}

func newDispatchAdmissionFixture(t *testing.T) *dispatchAdmissionFixture {
	return newDispatchAdmissionPolicyFixture(t, nil)
}

func TestDelegationAdmissionPartialMigrationReadinessIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	if err := f.service.CheckReady(f.f.ctx); err != nil {
		t.Fatal("complete schema refused", err)
	}
	if _, err := f.f.pool.Exec(f.f.ctx, `ALTER TABLE delegation_admission_snapshots DISABLE TRIGGER delegation_admission_immutable`); err != nil {
		t.Fatal(err)
	}
	if err := f.service.CheckReady(f.f.ctx); err == nil {
		t.Fatal("disabled immutable snapshot guard advertised ready")
	}
	if _, err := f.f.pool.Exec(f.f.ctx, `ALTER TABLE delegation_admission_snapshots ENABLE TRIGGER delegation_admission_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.pool.Exec(f.f.ctx, `ALTER TABLE delegation_admission_snapshots RENAME TO fixture_unavailable_admissions`); err != nil {
		t.Fatal(err)
	}
	if err := f.service.CheckReady(f.f.ctx); err == nil {
		t.Fatal("absent admission snapshot schema advertised ready")
	}
}
func newDispatchAdmissionPolicyFixture(t *testing.T, setup func(*delegationGuardFixture)) *dispatchAdmissionFixture {
	return newControlDispatchPolicyFixture(t, setup, false)
}

func newControlDispatchPolicyFixture(t *testing.T, setup func(*delegationGuardFixture), guarded bool) *dispatchAdmissionFixture {
	return newToolControlDispatchFixture(t, setup, guarded, false)
}
func newToolControlDispatchFixture(t *testing.T, setup func(*delegationGuardFixture), guarded, candidate bool) *dispatchAdmissionFixture {
	t.Helper()
	f := newDelegationGuardFixture(t)
	if setup != nil {
		setup(f)
	}
	manifestPath := filepath.Join(t.TempDir(), "dispatch-deletions.log")
	manifest := catalog.NewFileManifest(manifestPath)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	f.sources = catalog.New(f.pool, manifest)
	limits := task.DefaultDelegationLimits()
	limits.MaxParallel = 2
	sessions := session.NewPostgresRepository(f.pool)
	service, err := NewDelegationAdmission(f.pool, f.store, sessions, durableinput.NewStore(f.pool), f.sources, limits)
	if err != nil {
		t.Fatal(err)
	}
	const id = "delegate-parent-run"
	if _, err = f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,$2,'fixture','tool_pending','tool_pending','mode_1','fixture',clock_timestamp(),clock_timestamp(),$3,$4)`, id, f.parent.SessionKey, f.parent.TaskID, f.parent.Revision); err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.parent.SessionKey, RunID: id, OwnerID: "fixture-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: f.parent.SessionKey, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	var inputClaim durableinput.Record
	if guarded || candidate {
		inputs := durableinput.NewStore(f.pool)
		event := ingress.InputEvent{EventID: "guarded-request-input", IdempotencyKey: "guarded-request-input", SessionKey: f.parent.SessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"ask owner for a tab"}`, CreatedAt: time.Now().UTC()}
		if _, _, err = inputs.Accept(f.ctx, event, f.parent.OwnerID, f.parent.TaskID); err != nil {
			t.Fatal(err)
		}
		inputClaim, err = inputs.Claim(f.ctx, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		owned = durableinput.WithClaim(owned, inputClaim)
		if err = inputs.SaveCheckpoint(owned, id, "prepared", `{"UserMessage":"ask owner for a tab"}`); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.sources.PinTranscript(owned, f.parent.SessionKey, 0); err != nil {
		t.Fatal(err)
	}
	args := delegation.AdmissionArgs{ContractSource: f.source(t, "dispatch-contract", "user", f.parent.OwnerID, "Investigate selected DATA without tool permissions", nil), Sources: []delegation.SourceRef{f.source(t, "dispatch-data", "user", f.parent.OwnerID, "Selected bounded Russian DATA: проверить наблюдение", nil)}, Budget: delegation.Limits{ModelTurns: 2, Tokens: 10000, ExternalCalls: 1, ActiveMillis: 30000, EgressBytes: 1048576, ProviderInputBytes: 1048576}, Deadline: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond), Dependencies: []string{}}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	call := &toolbrokerv1.ToolCall{RunId: id, ToolCallId: "delegate-call", ToolName: delegation.AdmissionTool, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	contract := delegation.AdmissionContract()
	if guarded {
		call.ToolName, call.ToolCallId, call.ArgsJson = singletabcontrol.BindTool, "guarded-request-call", "{}"
		contract = singletabcontrol.RequestContract()
	}
	if candidate {
		raw, err = json.Marshal(map[string]any{"source_id": args.Sources[0].ID, "source_revision": args.Sources[0].Revision, "fact_key": "language", "value_json": `"ru"`, "summary": "Russian preference candidate"})
		if err != nil {
			t.Fatal(err)
		}
		call.ToolName, call.ToolCallId, call.ArgsJson = candidatecontrol.Tool, "memory-candidate-call", string(raw)
		contract = candidatecontrol.Contract()
	}
	repo := approvals.NewPostgresRepository(f.pool)
	repo.SetCheckpointHooks(boundaryCheckpointHooks{}) // No authority: lifecycle checkpoints have their own fault suite.
	approvalService := approvals.NewService(repo, nil)
	reader := &actionContextReader{runs: run.NewService(run.NewPostgresRepository(f.pool), nil), sessions: sessions, leases: f.leases, tasks: f.store, approvals: approvalService, pool: f.pool, delegationReady: service.Manager != nil}
	reader.guardedTabRequestReady = guarded
	reader.memoryCandidateReady = candidate
	journal := actionauth.NewPostgresStore(f.pool)
	journal.SetBudgets(f.budgets)
	journal.SetRunDataGuard(f.sources.CheckRunSourcesTx)
	auth := actionauth.NewService(journal, reader, nil)
	prepared, err := auth.Prepare(f.ctx, call, contract)
	if err != nil || !prepared.RequiresApproval {
		t.Fatal("exact prepare approval required", err)
	}
	expires := time.Now().Add(45 * time.Second).UTC().Truncate(time.Microsecond)
	approval, err := approvalService.CreatePendingApproval(owned, approvals.CreatePendingParams{RunID: id, SessionKey: f.parent.SessionKey, ToolCallID: call.ToolCallId, ToolName: call.ToolName, ArgsJSON: call.ArgsJson, RequestedVia: "web", ActionBinding: prepared.Binding, ContinuationJSON: `{"version":1}`, Summary: prepared.Summary, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	actor := approvals.WithActor(f.ctx, approvals.Actor{OwnerID: f.parent.OwnerID, Channel: "web", SessionKey: f.parent.SessionKey, DeliveryID: "delegate-decision"})
	if _, _, err = approvalService.ResolveByApprovalID(actor, approvals.ResolveByApprovalIDParams{ApprovalID: approval.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: f.parent.OwnerID, ResolvedBy: f.parent.OwnerID}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET status='tool_running',current_state='tool_running' WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	grant, err := auth.Authorize(f.ctx, call, contract, prepared.Binding.ActionID, approval.ApprovalID, "")
	if err != nil {
		t.Fatal(err)
	}
	return &dispatchAdmissionFixture{f: f, service: service, reader: reader, approvals: approvalService, journal: journal, auth: auth, owned: owned, args: args, call: call, prepared: prepared, grant: grant, envelope: delegation.AdmissionEnvelope{RunID: id, ToolCallID: call.ToolCallId, EffectID: prepared.EffectID, ArgsJSON: call.ArgsJson, ContractHash: prepared.Binding.ContractHash}, manifestPath: manifestPath, inputClaim: inputClaim}
}

func (f *dispatchAdmissionFixture) claim(t *testing.T) {
	t.Helper()
	if err := f.journal.ClaimGrant(f.f.ctx, f.grant.ID); err != nil {
		t.Fatal(err)
	}
}
func (f *dispatchAdmissionFixture) noAdmission(t *testing.T) {
	t.Helper()
	for _, table := range []string{"task_delegations", "delegation_admission_snapshots"} {
		var n int
		if err := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("denied admission left %s rows: %d %v", table, n, err)
		}
	}
	var childInputs int
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM accepted_inputs WHERE task_id<>$1`, f.f.parent.TaskID).Scan(&childInputs); err != nil || childInputs != 0 {
		t.Fatal("denial published child input", childInputs, err)
	}
	var n int
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM tasks WHERE parent_task_id=$1`, f.f.parent.TaskID).Scan(&n); err != nil || n != 0 {
		t.Fatal("denial published child task", n, err)
	}
}

func TestDelegationAdmissionTokenIsNotAuthorityIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	const token = "private-delegation-fixture-token-long-value"
	h, err := delegation.NewAdmissionHandler(token, f.service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	client, _ := delegation.NewAdmissionClient(server.URL, token)
	if _, err = client.AdmitDispatched(f.f.ctx, f.envelope); err == nil {
		t.Fatal("token and unused grant admitted child")
	}
	f.noAdmission(t)
	f.claim(t)
	for _, mutate := range []func(*delegation.AdmissionEnvelope){
		func(e *delegation.AdmissionEnvelope) { e.EffectID = "other-effect" },
		func(e *delegation.AdmissionEnvelope) { e.RunID = "other-run" },
		func(e *delegation.AdmissionEnvelope) { e.ToolCallID = "other-call" },
		func(e *delegation.AdmissionEnvelope) { e.ContractHash = strings.Repeat("b", 64) },
		func(e *delegation.AdmissionEnvelope) {
			e.ArgsJSON = strings.Replace(e.ArgsJSON, `"tokens":10000`, `"tokens":10001`, 1)
		},
		func(e *delegation.AdmissionEnvelope) {
			e.ArgsJSON = strings.TrimSuffix(e.ArgsJSON, "}") + `,"owner_id":"forged"}`
		},
		func(e *delegation.AdmissionEnvelope) {
			e.ArgsJSON = strings.TrimSuffix(e.ArgsJSON, "}") + `,"permissions":[{"tool":"*"}]}`
		},
	} {
		e := f.envelope
		mutate(&e)
		if _, err = client.AdmitDispatched(f.f.ctx, e); err == nil {
			t.Fatal("changed dispatch envelope admitted child")
		}
		f.noAdmission(t)
	}
	if _, err = f.service.AdmitDispatched(f.f.owner, f.envelope); err == nil {
		t.Fatal("owner context could impersonate claimed Broker worker")
	}
	receipt, err := client.AdmitDispatched(f.f.ctx, f.envelope)
	if err != nil || receipt.TaskID == "" || receipt.State != "queued" || receipt.InputID == "" {
		t.Fatal(receipt, err)
	}
	replay, err := client.AdmitDispatched(f.f.ctx, f.envelope)
	if err != nil || replay != receipt {
		t.Fatal("current inflight exact retry duplicated admission", replay, err)
	}
	var n int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM task_delegations`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE delegation_admission_snapshots SET args_hash=$2 WHERE task_id=$1`, receipt.TaskID, strings.Repeat("b", 64)); err == nil {
		t.Fatal("accepted capability snapshot was mutable")
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `DELETE FROM delegation_admission_snapshots WHERE task_id=$1`, receipt.TaskID); err == nil {
		t.Fatal("accepted capability snapshot was deletable")
	}
	var snapshot []byte
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT accepted_json FROM delegation_admission_snapshots WHERE task_id=$1`, receipt.TaskID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snapshot), "Selected bounded Russian") || strings.Contains(string(snapshot), "Investigate selected DATA") {
		t.Fatal("raw source prose copied into durable authority")
	}
}

func TestDelegationAdmissionCurrentSourceAndRevisionIntegration(t *testing.T) {
	for _, kind := range []string{"source", "deletion", "revision", "terminal_run", "lease", "expiry", "autonomy"} {
		t.Run(kind, func(t *testing.T) {
			f := newDispatchAdmissionFixture(t)
			f.claim(t)
			switch kind {
			case "source":
				if _, err := f.f.sources.Put(f.f.owner, catalog.PutSource{ExternalKey: "dispatch-data", Kind: "model_candidate", ScopeType: "user", ScopeID: f.f.parent.OwnerID, ExpectedRevision: f.args.Sources[0].Revision, Title: "Corrected", Body: "owner correction"}); err != nil {
					t.Fatal(err)
				}
			case "deletion":
				if _, err := f.f.sources.Delete(f.f.owner, f.args.Sources[0].ID, "delete", f.args.Sources[0].Revision); err != nil {
					t.Fatal(err)
				}
			case "autonomy":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET autonomy_mode='mode_0' WHERE run_id=$1`, f.call.RunId); err != nil {
					t.Fatal(err)
				}
			case "revision":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1 WHERE task_id=$1`, f.f.parent.TaskID); err != nil {
					t.Fatal(err)
				}
			case "terminal_run":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='completed',current_state='completed' WHERE run_id=$1`, f.call.RunId); err != nil {
					t.Fatal(err)
				}
			case "lease":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE run_id=$1`, f.call.RunId); err != nil {
					t.Fatal(err)
				}
			case "expiry":
				if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE action_authorizations SET expires_at=clock_timestamp()-interval '1 second' WHERE authorization_id=$1`, f.grant.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); err == nil {
				t.Fatal("stale dispatch admitted child")
			}
			f.noAdmission(t)
		})
	}
}

func TestDelegationAdmissionDurableLifecycleWithoutParentProcessIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	receipt, err := f.service.AdmitDispatched(f.f.ctx, f.envelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE runs SET status='completed',current_state='completed' WHERE run_id=$1`, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE run_id=$1`} {
		if _, err = f.f.pool.Exec(f.f.ctx, q, f.call.RunId); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE action_authorizations SET state='dispatch_completed',expires_at=clock_timestamp()-interval '1 second' WHERE authorization_id=$1`, f.grant.ID); err != nil {
		t.Fatal(err)
	}
	// The same module is reconstructed as after restart: lifecycle reads the
	// immutable child snapshot/current authority, without using the old grant.
	restarted, err := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DelegationLimits{MaxDepth: 3, MaxChildren: 8, MaxParallel: 2, MaxDependencies: 8})
	if err != nil {
		t.Fatal(err)
	}
	f.f.manager = restarted.Manager
	if _, err = restarted.Manager.Advance(f.f.ctx, f.f.parent.TaskID); err != nil {
		t.Fatal("completed parent process invalidated accepted work", err)
	}
	child, err := f.f.store.Get(f.f.ctx, receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := f.f.worker(t, child, "accepted-child-run")
	selected, err := f.f.sources.ReadDelegatedRun(ctx, "accepted-child-run")
	if err != nil || selected == nil || len(selected.Sources) != 2 || len(selected.Contract.Permissions) != 0 || !selected.Contract.Deadline.After(f.grant.ExpiresAt) {
		t.Fatal("accepted child lost exact context or inherited lease deadline", selected, err)
	}
	if err = f.f.sources.PinTranscript(ctx, child.SessionKey, 0); err != nil {
		t.Fatal(err)
	}
	reader := *f.reader
	if c, err := reader.GetActionContext(f.f.ctx, "accepted-child-run"); err != nil || c.Delegation == nil || len(c.Delegation.Permissions) != 0 {
		t.Fatal(c, err)
	}
	if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending' WHERE run_id='accepted-child-run'`); err != nil {
		t.Fatal(err)
	}
	call := &toolbrokerv1.ToolCall{RunId: "accepted-child-run", ToolCallId: "widen-child", ToolName: delegation.AdmissionTool, ArgsJson: f.call.ArgsJson, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	if _, err = actionauth.NewService(f.journal, &reader, nil).Prepare(f.f.ctx, call, delegation.AdmissionContract()); err == nil {
		t.Fatal("context-only child acquired delegate tool")
	}
	// Admission was real budgeted work, and child allocation remains bound to
	// the shared root/delegation ledger rather than a reset fresh session budget.
	var raw []byte
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT used_json FROM execution_budget_scopes WHERE scope_id=$1`, budget.ScopeID("task", child.OwnerID, f.f.parent.TaskID)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var units budget.Units
	if err = json.Unmarshal(raw, &units); err != nil || units.Children != 1 || units.ExternalCalls != 1 {
		t.Fatal("shared budget did not charge actual admission", units, err)
	}
}

func TestDelegationAdmissionWaitCrossesGrantDeadlineIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	// Two connections: hold the actual dispatch row while admission uses the
	// other. Advance the exact persisted grant deadline, preserving JSON/column
	// agreement; it must recheck database time after the lock wait.
	tx, err := f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.f.ctx)
	var actual actionauth.Grant
	actual = f.grant
	actual.ExpiresAt = time.Now().Add(350 * time.Millisecond).UTC().Truncate(time.Microsecond)
	raw, _ := json.Marshal(actual)
	if _, err = tx.Exec(f.f.ctx, `UPDATE action_authorizations SET expires_at=$2,grant_data=$3 WHERE authorization_id=$1`, actual.ID, actual.ExpiresAt, raw); err != nil {
		t.Fatal(err)
	}
	// Publish deadline before acquiring the wait barrier.
	if err = tx.Commit(f.f.ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.f.ctx, `SELECT 1 FROM action_effect_dispatches WHERE authorization_id=$1 FOR UPDATE`, actual.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); done <- err }()
	select {
	case err = <-done:
		t.Fatal("admission did not wait for dispatch lock", err)
	case <-time.After(450 * time.Millisecond):
	}
	if err = tx.Commit(f.f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("expired grant admitted child after wait")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("two-connection admission deadlocked")
	}
	f.noAdmission(t)
}

func TestDelegationAdmissionReplayRechecksDispatchAfterWaitIntegration(t *testing.T) {
	for _, kind := range []string{"expiry", "dispatch_transition"} {
		t.Run(kind, func(t *testing.T) {
			f := newDispatchAdmissionFixture(t)
			f.claim(t)
			receipt, err := f.service.AdmitDispatched(f.f.ctx, f.envelope)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second).UTC().Truncate(time.Microsecond)
			if kind == "expiry" {
				actual := f.grant
				actual.ExpiresAt = deadline
				raw, _ := json.Marshal(actual)
				if _, err = f.f.pool.Exec(f.f.ctx, `UPDATE action_authorizations SET expires_at=$2,grant_data=$3 WHERE authorization_id=$1`, actual.ID, deadline, raw); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := f.f.pool.Begin(f.f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(f.f.ctx)
			var holderPID int
			if err = tx.QueryRow(f.f.ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(f.f.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('butler-task-tree:'||$1,0))`, f.f.parent.TaskID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); done <- err }()
			// Prove the unlocked dispatch read succeeded and the real manager is
			// now waiting on this tree barrier, using the holder's same connection.
			waitUntil := time.Now().Add(1500 * time.Millisecond)
			blocked := false
			for !blocked && time.Now().Before(waitUntil) {
				if _, err = tx.Exec(f.f.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
					t.Fatal(err)
				}
				if err = tx.QueryRow(f.f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%pg_advisory_xact_lock%')`, holderPID).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if !blocked {
					select {
					case err = <-done:
						t.Fatal("replay did not reach admission tree barrier", err)
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			if !blocked {
				t.Fatal("replay did not wait on the actual tree lock")
			}
			if kind == "expiry" {
				time.Sleep(time.Until(deadline.Add(100 * time.Millisecond)))
			} else if _, err = tx.Exec(f.f.ctx, `UPDATE action_authorizations SET state='dispatch_completed' WHERE authorization_id=$1`, f.grant.ID); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(f.f.ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err == nil {
					t.Fatal("stale dispatch replay returned an accepted receipt after wait")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("two-connection replay deadlocked")
			}
			var count int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM delegation_admission_snapshots WHERE task_id=$1`, receipt.TaskID).Scan(&count); err != nil || count != 1 {
				t.Fatal("denied replay altered accepted history", count, err)
			}
			if _, err = f.service.Manager.Advance(f.f.ctx, f.f.parent.TaskID); err != nil {
				t.Fatal("admission replay check leaked into durable maintenance", err)
			}
		})
	}
}

func TestDelegationAdmissionReplayVersusActualOutcomeIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	receipt, err := f.service.AdmitDispatched(f.f.ctx, f.envelope)
	if err != nil {
		t.Fatal(err)
	}
	// A second real pool2 models the independent Broker journal writer. The
	// fixture trigger only pauses evidence insertion after its effect lock;
	// RecordDispatchOutcome performs all actual outcome/evidence/budget writes.
	other, err := pgxpool.NewWithConfig(f.f.ctx, f.f.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err = f.f.pool.Exec(f.f.ctx, `CREATE FUNCTION fixture_pause_admission_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(hashtextextended('delegation-outcome:'||current_schema(),0)); RETURN NEW; END $$;
 CREATE TRIGGER fixture_pause_admission_ack BEFORE INSERT ON action_effect_evidence FOR EACH ROW EXECUTE FUNCTION fixture_pause_admission_ack()`); err != nil {
		t.Fatal(err)
	}
	tx, err := f.f.pool.Begin(f.f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.f.ctx)
	var holderPID int
	if err = tx.QueryRow(f.f.ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(f.f.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('delegation-outcome:'||current_schema(),0))`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(receipt)
	digest, _ := actionauth.ArgumentsHash(string(raw))
	now := time.Now().UTC().Truncate(time.Microsecond)
	evidence := actionauth.EffectEvidence{EvidenceID: "admission-ack:" + receipt.TaskID, ActionID: f.prepared.EffectID, SourceActionID: f.prepared.Binding.ActionID, SourceRunID: f.call.RunId, SourceToolCallID: f.call.ToolCallId, Source: "runtime_response", Method: "local_runtime_ack", Verdict: actionauth.StateConfirmed, ResultDigest: digest, NormalizedResultDigest: digest, ObservedAt: now, RecordedAt: now}
	outcomeJournal := actionauth.NewPostgresStore(other)
	outcomeJournal.SetBudgets(budget.NewController(other, f.f.budgets.Policy))
	outcomeDone := make(chan error, 1)
	go func() {
		_, err := outcomeJournal.RecordDispatchOutcome(f.f.ctx, f.grant.ID, "dispatch_completed", evidence)
		outcomeDone <- err
	}()
	var outcomePID int
	until := time.Now().Add(2 * time.Second)
	for outcomePID == 0 && time.Now().Before(until) {
		if _, err = tx.Exec(f.f.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err = tx.QueryRow(f.f.ctx, `SELECT COALESCE((SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%INSERT INTO action_effect_evidence%' LIMIT 1),0)`, holderPID).Scan(&outcomePID); err != nil {
			t.Fatal(err)
		}
		if outcomePID == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if outcomePID == 0 {
		select {
		case err = <-outcomeDone:
			t.Fatal("actual outcome returned before the evidence barrier", err)
		default:
		}
		t.Fatal("actual outcome writer did not reach the evidence barrier")
	}
	replayDone := make(chan error, 1)
	go func() { _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); replayDone <- err }()
	blocked := false
	until = time.Now().Add(2 * time.Second)
	for !blocked && time.Now().Before(until) {
		if _, err = tx.Exec(f.f.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err = tx.QueryRow(f.f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, outcomePID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if !blocked {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !blocked {
		t.Fatal("admission did not wait behind the actual outcome effect lock")
	}
	if err = tx.Commit(f.f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-outcomeDone:
		if err != nil {
			t.Fatal("actual outcome failed or deadlocked with admission", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("outcome writer deadlocked with admission")
	}
	select {
	case err = <-replayDone:
		if err == nil {
			t.Fatal("spent dispatch replay returned an admission receipt")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admission deadlocked with outcome writer")
	}
	effect, err := f.journal.GetEffect(f.f.ctx, f.prepared.EffectID)
	if err != nil || effect.State != actionauth.StateConfirmed || len(effect.EvidenceRefs) != 1 || effect.EvidenceRefs[0] != evidence.EvidenceID {
		t.Fatal("confirmed actual admission evidence lost", effect, err)
	}
	var count int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM delegation_admission_snapshots WHERE task_id=$1`, receipt.TaskID).Scan(&count); err != nil || count != 1 {
		t.Fatal("outcome race duplicated or erased child", count, err)
	}
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM execution_loop_observations WHERE effect_id=$1 AND result_digest=$2`, f.prepared.EffectID, digest).Scan(&count); err != nil || count != 1 {
		t.Fatal("actual outcome omitted shared loop observation", count, err)
	}
}

type failingAdmissionInput struct{}

func (failingAdmissionInput) AcceptBoundedTx(context.Context, pgx.Tx, delegation.InputRequest) (string, error) {
	return "", delegation.ErrDenied
}
func TestDelegationAdmissionQueueFailureRollsBackIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	service, err := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), failingAdmissionInput{}, f.f.sources, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdmitDispatched(f.f.ctx, f.envelope); err == nil {
		t.Fatal("queue failure committed admission")
	}
	f.noAdmission(t)
	var count int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed admission leaked child session", count, err)
	}
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'child:%'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed admission charged child allocation", count, err)
	}
}

func TestDelegationAdmissionAnonymousHTTPIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	h, _ := delegation.NewAdmissionHandler("private-delegation-fixture-token-long-value", f.service)
	raw, _ := json.Marshal(f.envelope)
	r := httptest.NewRequest(http.MethodPost, "/internal/delegate", strings.NewReader(string(raw)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	f.noAdmission(t)
}

func TestDelegationAdmissionOwnerStopRetainsDispatchIntegration(t *testing.T) {
	f := newDispatchAdmissionFixture(t)
	f.claim(t)
	controls := task.NewController(f.f.store, f.f.leases.Authority(), f.f.leases, run.NewPostgresRepository(f.f.pool), approvals.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), transcript.NewStore(f.f.pool))
	if _, err := controls.Apply(f.f.owner, task.ControlRequest{TaskID: f.f.parent.TaskID, ExpectedRevision: f.f.parent.Revision, Kind: "stop", Instruction: "stop current independent research admission"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); err == nil {
		t.Fatal("stopped parent admitted new child")
	}
	f.noAdmission(t)
	effect, err := f.journal.GetEffect(f.f.ctx, f.prepared.EffectID)
	if err != nil || effect.State != actionauth.StateExecuting {
		t.Fatal("stop invented an undo or erased irreversible claim", effect, err)
	}
}

func TestDelegationAdmissionAssignedMandateRevocationIntegration(t *testing.T) {
	for _, revokeFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "admitted_then_paused", true: "paused_before_admission"}[revokeFirst], func(t *testing.T) {
			var manager *goal.Manager
			var objective actionauth.GoalPolicy
			setup := func(f *delegationGuardFixture) {
				manager = goal.New(f.pool, f.store, nil)
				objective = actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Current owner research objective", Purpose: "delegate_research", ExpiresAt: time.Now().Add(24 * time.Hour)}
				r, err := manager.PutGoal(f.owner, goal.GoalRequest{Goal: objective})
				if err != nil {
					t.Fatal(err)
				}
				objective.ID, objective.Revision = r.SubjectID, r.Revision
				a := f.caps.Budget
				actor := func(id string) context.Context {
					return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "web", DeliveryID: id})
				}
				m, err := manager.PutMandate(actor("assigned-delegate-mandate"), goal.MandateRequest{Mandate: actionauth.Mandate{GoalID: objective.ID, GoalRevision: objective.Revision, Status: "active", Purpose: objective.Purpose, Tools: []string{delegation.AdmissionTool}, ResourceScopes: []actionauth.ResourceScope{{Origin: "tool:" + delegation.AdmissionTool}}, Effects: []actionauth.Effect{actionauth.EffectPrepare}, Privacy: "local_only", Budget: actionauth.MandateBudget{ModelTurns: a.ModelTurns, Tokens: a.Tokens, ExternalCalls: a.ExternalCalls, Children: a.Children, ActiveMillis: a.ActiveMillis, EgressBytes: a.EgressBytes, ProviderInputBytes: a.ProviderInputBytes}, ExpiresAt: time.Now().Add(2 * time.Hour), Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}})
				if err != nil {
					t.Fatal(err)
				}
				parent, err := manager.CreateOwnerTask(actor("assigned-delegate-task"), goal.TaskRequest{SessionKey: f.parent.SessionKey, GoalID: objective.ID, MandateID: m.SubjectID, Purpose: objective.Purpose, Goal: "Scoped independent research", CompletionCriteria: []string{"independent evidence"}, InitiativeReason: "explicit owner work", ObservationRef: "fixture:owner"})
				if err != nil {
					t.Fatal(err)
				}
				f.parent = parent
			}
			f := newDispatchAdmissionPolicyFixture(t, setup)
			f.claim(t)
			var receipt delegation.AdmissionReceipt
			if !revokeFirst {
				var err error
				receipt, err = f.service.AdmitDispatched(f.f.ctx, f.envelope)
				if err != nil {
					t.Fatal("current assigned mandate denied exact delegate", err)
				}
			}
			objective.Status = "paused"
			owner := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "pause-assigned-research"})
			if _, err := manager.PutGoal(owner, goal.GoalRequest{Goal: objective, ExpectedRevision: objective.Revision}); err != nil {
				t.Fatal(err)
			}
			if revokeFirst {
				if _, err := f.service.AdmitDispatched(f.f.ctx, f.envelope); err == nil {
					t.Fatal("paused goal admitted child before sweep")
				}
				f.noAdmission(t)
			} else {
				child, err := f.f.store.Get(f.f.ctx, receipt.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				tx, err := f.f.pool.Begin(f.f.ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(f.f.ctx)
				c, err := delegation.LoadTx(f.f.ctx, tx, child.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				guards, _ := NewDelegationGuards(f.f.sources, f.service.reader)
				if err = guards.CheckLifecycleTx(f.f.ctx, tx, f.f.parent, child, *c); err == nil {
					t.Fatal("accepted snapshot alone survived current owner revoke")
				}
			}
		})
	}
}
