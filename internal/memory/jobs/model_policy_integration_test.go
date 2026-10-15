package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
)

func seedModelTasks(t *testing.T, w *ModelWorker, ctx context.Context) {
	t.Helper()
	_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('t','s','owner','completed','completed'),('t2','s','owner','completed','completed')`)
	if e != nil {
		t.Fatal(e)
	}
}
func seedModelMandate(t *testing.T, w *ModelWorker, ctx context.Context, privacy string, turns int64) actionauth.Mandate {
	t.Helper()
	seedModelTasks(t, w, ctx)
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	m := actionauth.Mandate{ID: "mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "memory", Privacy: privacy, ExpiresAt: expiry, BudgetDeadline: expiry, Budget: actionauth.MandateBudget{ModelTurns: turns, Tokens: 10000, ActiveMillis: 10000, ProviderInputBytes: 4 * 8388608}}
	g := actionauth.GoalPolicy{ID: "goal", OwnerID: "owner", Kind: "objective", Status: "active", Revision: 1, Purpose: "memory", ExpiresAt: expiry}
	_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2,'{}')`, expiry, jsonBytes(g))
	if e != nil {
		t.Fatal(e)
	}
	_, e = w.Jobs.pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1,$2,'owner_goal','goal',1,'active')`, jsonBytes(m), expiry)
	if e != nil {
		t.Fatal(e)
	}
	_, e = w.Jobs.pool.Exec(ctx, `INSERT INTO owner_purpose_policies(owner_id,purpose,revision) VALUES('owner','memory',0); INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('t','owner','goal',1,'mandate',1,'memory','owner','fixture'),('t2','owner','goal',1,'mandate',1,'memory','owner','fixture')`)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestModelDifferentJobsShareFinalBudgetUnitIntegration(t *testing.T) {
	for _, dimension := range []string{"owner_day", "task", "mandate"} {
		t.Run(dimension, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			switch dimension {
			case "owner_day":
				w.Jobs.budgets.Policy.Day.ModelTurns = 1
			case "task":
				seedModelTasks(t, w, ctx)
				w.Jobs.budgets.Policy.Task.ModelTurns = 1
			case "mandate":
				seedModelMandate(t, w, ctx, "cloud_allowed", 1)
			}
			for i, id := range []string{"policy-last-a", "policy-last-b"} {
				q := policyRequest(src)
				q.ID = id
				if dimension == "task" {
					q.Job.TaskID = "t"
				}
				if dimension == "mandate" {
					q.Job.TaskID = []string{"t", "t2"}[i]
				}
				if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
					t.Fatal(e)
				}
				if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
					t.Fatal(e)
				}
			}
			out := make(chan error, 2)
			var wg sync.WaitGroup
			for range 2 {
				wg.Add(1)
				go func() { defer wg.Done(); _, e := w.ProcessOne(ctx); out <- e }()
			}
			wg.Wait()
			close(out)
			rejected := 0
			for e := range out {
				if e != nil {
					var ex *budget.ExhaustedError
					if !errors.As(e, &ex) || ex.Kind != dimension {
						t.Fatal(e)
					}
					rejected++
				}
			}
			if rejected != 1 || w.Provider.(*modelFixtureProvider).calls.Load() != 1 {
				t.Fatal("shared last unit", rejected, w.Provider.(*modelFixtureProvider).calls.Load())
			}
		})
	}
}
func TestModelMandateAndSourceRevocationDuringCallIntegration(t *testing.T) {
	for _, mutation := range []string{"mandate_revoke", "mandate_revision", "goal_revision", "source_revoke"} {
		t.Run(mutation, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			seedModelMandate(t, w, ctx, "cloud_allowed", 4)
			q := policyRequest(src)
			q.Job.TaskID = "t"
			j := schedule(t, w, ctx, q)
			w.Provider.(*modelFixtureProvider).callback = func() {
				var e error
				switch mutation {
				case "mandate_revoke":
					_, e = w.Jobs.pool.Exec(ctx, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id='mandate'`)
				case "mandate_revision":
					_, e = w.Jobs.pool.Exec(ctx, `UPDATE action_mandates SET revision=revision+1 WHERE mandate_id='mandate'`)
				case "goal_revision":
					_, e = w.Jobs.pool.Exec(ctx, `UPDATE owner_goals SET revision=revision+1 WHERE goal_id='goal'`)
				case "source_revoke":
					_, e = w.Jobs.catalog.Delete(ctx, src.ID, "revoke", src.Revision)
				}
				if e != nil {
					t.Fatal(e)
				}
			}
			if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
				t.Fatal("revoked authority published")
			}
			var n int
			if e := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts WHERE state='published'`).Scan(&n); e != nil || n != 0 {
				t.Fatal(n, e)
			}
		})
	}
}
func TestModelTaskSourceCannotDropLocalOnlyBindingIntegration(t *testing.T) {
	w, ctx, _ := modelFixture(t)
	seedModelMandate(t, w, ctx, "local_only", 4)
	src, e := w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "local-task", Kind: "document", ScopeType: "task", ScopeID: "t", Title: "private", Body: "local data"})
	if e != nil {
		t.Fatal(e)
	}
	for _, taskID := range []string{"", "t", "t2"} {
		q := policyRequest(src)
		q.Job.TaskID = taskID
		if _, e = w.Jobs.CreateModelPolicy(ctx, q); e == nil {
			t.Fatal("local task source escaped", taskID)
		}
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
		t.Fatal("cloud call")
	}
}
func TestModelPolicyPauseKeepsJobsAndAttemptsIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	q := policyRequest(src)
	if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.ControlModel(ctx, q.ID, "paused"); e != nil {
		t.Fatal(e)
	}
	for range 8 {
		worked, e := w.ProcessOne(ctx)
		if e != nil || worked {
			t.Fatal(worked, e)
		}
	}
	var attempts int
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT attempts FROM memory_aux_jobs WHERE policy_id=$1`, q.ID).Scan(&attempts); e != nil || attempts != 0 {
		t.Fatal(attempts, e)
	}
	if e := w.Jobs.ControlModel(ctx, q.ID, "active"); e != nil {
		t.Fatal(e)
	}
	if worked, e := w.ProcessOne(ctx); e != nil || !worked {
		t.Fatal(worked, e)
	}
}
func TestModelBoundedTriggersQueueAndExpiredPendingIntegration(t *testing.T) {
	for _, trigger := range []string{"schedule", "session_inactive"} {
		t.Run(trigger, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			q := policyRequest(src)
			q.Trigger = trigger
			q.MaxJobs = 1
			if trigger == "session_inactive" {
				_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('r','s','i','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes')`)
				if e != nil {
					t.Fatal(e)
				}
				q.Job.OriginalRunID = "r"
			}
			if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
				t.Fatal(e)
			}
			for range 5 {
				if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
					t.Fatal(e)
				}
			}
			var n int
			if e := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_aux_jobs WHERE policy_id=$1`, q.ID).Scan(&n); e != nil || n != 1 {
				t.Fatal(n, e)
			}
		})
	}
	t.Run("queue_backpressure", func(t *testing.T) {
		w, ctx, src := modelFixture(t)
		q := policyRequest(src)
		if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
			t.Fatal(e)
		}
		if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
			t.Fatal(e)
		}
		_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,kind,purpose,task_id,task_revision,constraints_revision,original_run_id,authority,sources,source_fingerprint,allowance,deadline,policy_id) SELECT job_id||':'||n,owner_id,kind,purpose,task_id,task_revision,constraints_revision,original_run_id,authority,sources,source_fingerprint,allowance,deadline,policy_id FROM memory_aux_jobs CROSS JOIN generate_series(1,63) n`)
		if e != nil {
			t.Fatal(e)
		}
		q2 := q
		q2.ID = "policy-overflow"
		if _, e = w.Jobs.CreateModelPolicy(ctx, q2); e != nil {
			t.Fatal(e)
		}
		if e = w.Jobs.scheduleModelPolicy(ctx, q2.ID); e != nil {
			t.Fatal(e)
		}
		var reason string
		if e = w.Jobs.pool.QueryRow(ctx, `SELECT reason FROM memory_model_policies WHERE policy_id=$1`, q2.ID).Scan(&reason); e != nil || reason != "queue_full" {
			t.Fatal(reason, e)
		}
	})
}

func TestModelActualUsageOverrunRetainedIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	j := schedule(t, w, ctx, policyRequest(src))
	w.Provider.(*modelFixtureProvider).overrun = true
	if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
		t.Fatal("overrun published")
	}
	var output, count int
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT (usage->>'OutputTokens')::int,(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate') FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&output, &count); e != nil || output != 80 || count != 0 {
		t.Fatal(output, count, e)
	}
}
func TestModelPriorityAtDispatchAndDuringInferenceIntegration(t *testing.T) {
	for _, phase := range []string{"before_dispatch", "during_call"} {
		t.Run(phase, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			j := schedule(t, w, ctx, policyRequest(src))
			p := w.Provider.(*modelFixtureProvider)
			insert := func() {
				_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('interactive','owner','web'); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('interactive','interactive-run','lease',1,clock_timestamp()+INTERVAL '1 minute')`)
				if e != nil {
					t.Fatal(e)
				}
			}
			if phase == "before_dispatch" {
				p.beforeDispatch = insert
			} else {
				p.callback = func() { insert(); time.Sleep(450 * time.Millisecond) }
			}
			if e := w.execute(ctx, j, QuoteProbe{}); !errors.Is(e, ErrWaiting) {
				t.Fatal(e)
			}
			var attempts int
			var state string
			if e := w.Jobs.pool.QueryRow(ctx, `SELECT attempts,status FROM memory_aux_jobs WHERE job_id=$1`, j.ID).Scan(&attempts, &state); e != nil || state != "pending" {
				t.Fatal(attempts, state, e)
			}
			expected := 0
			if phase == "during_call" {
				expected = 1
			}
			if attempts != expected {
				t.Fatal("retry count", attempts, expected)
			}
			p.beforeDispatch = nil
			p.callback = nil
			if _, e := w.Jobs.pool.Exec(ctx, `DELETE FROM session_ownership WHERE session_key='interactive'`); e != nil {
				t.Fatal(e)
			}
			_, e := w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id=$1`, j.ID)
			if e != nil {
				t.Fatal(e)
			}
			if worked, e := w.ProcessOne(ctx); e != nil || !worked {
				t.Fatal(worked, e)
			}
		})
	}
}
func TestModelSessionPrivacyRecheckedAtPublicationIntegration(t *testing.T) {
	w, ctx, _ := modelFixture(t)
	_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('r','s','i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
	if e != nil {
		t.Fatal(e)
	}
	src, e := w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "session-data", Kind: "document", ScopeType: "session", ScopeID: "s", Title: "source", Body: "source"})
	if e != nil {
		t.Fatal(e)
	}
	p := w.Provider.(*modelFixtureProvider)
	p.source = src
	q := policyRequest(src)
	q.Job.OriginalRunID = "r"
	j := schedule(t, w, ctx, q)
	p.callback = func() {
		_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('conflict','s','owner','other task')`)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = w.execute(ctx, j, QuoteProbe{}); e == nil {
		t.Fatal("changed privacy predicate published")
	}
}

func TestModelClaimExpiryAfterBudgetWaitIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	j := schedule(t, w, ctx, policyRequest(src))
	var until time.Time
	if e := w.Jobs.pool.QueryRow(ctx, `UPDATE memory_aux_jobs SET claim_until=clock_timestamp()+INTERVAL '300 milliseconds' WHERE job_id=$1 RETURNING claim_until`, j.ID).Scan(&until); e != nil {
		t.Fatal(e)
	}
	j.ClaimUntil = &until
	now := time.Now().UTC()
	day := now.Truncate(24 * time.Hour)
	scope, e := w.Jobs.budgets.Ledger.Ensure(ctx, budget.ScopeSpec{Kind: "owner_day", OwnerID: "owner", SubjectID: day.Format("2006-01-02"), Limits: w.Jobs.budgets.Policy.Day, Deadline: day.Add(24 * time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	held, e := w.Jobs.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer held.Rollback(ctx)
	if _, e = held.Exec(ctx, `SELECT scope_id FROM execution_budget_scopes WHERE scope_id=$1 FOR UPDATE`, scope.ID); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, _, e := w.Admit(ctx, j); result <- e }()
	time.Sleep(450 * time.Millisecond)
	if e = held.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-result:
		if e == nil {
			t.Fatal("expired after allowance wait")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admission did not finish")
	}
	var count int
	if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count); e != nil || count != 0 {
		t.Fatal("failed transaction charged", count, e)
	}
}

func TestModelTransientPublicationRollbackRecoveryIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	j := schedule(t, w, ctx, policyRequest(src))
	_, e := w.Jobs.pool.Exec(ctx, `CREATE SEQUENCE issue82_fault;CREATE FUNCTION issue82_publication_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_kind='model_candidate' AND nextval('issue82_fault')=1 THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='injected rollback'; END IF; RETURN NEW; END $$;CREATE TRIGGER issue82_publication_fault AFTER INSERT ON memory_sources FOR EACH ROW EXECUTE FUNCTION issue82_publication_fault()`)
	if e != nil {
		t.Fatal(e)
	}
	if e = w.execute(ctx, j, QuoteProbe{}); e == nil {
		t.Fatal("fault not observed")
	}
	if _, e = w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	if worked, e := w.ProcessOne(ctx); e != nil || !worked {
		t.Fatal(worked, e)
	}
	var attempts, outputs int
	if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*),(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate') FROM memory_model_attempts`).Scan(&attempts, &outputs); e != nil || attempts != 2 || outputs != 1 {
		t.Fatal(attempts, outputs, e)
	}
}

func TestModelLocalWaitAndPolicyDeadlineIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	q := policyRequest(src)
	q.Privacy = "local_only"
	q.Job.Deadline = time.Now().Add(700 * time.Millisecond)
	j := schedule(t, w, ctx, q)
	if e := w.execute(ctx, j, QuoteProbe{}); !errors.Is(e, ErrWaiting) {
		t.Fatal(e)
	}
	var state, reason string
	var attempts int
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT status,error_code,attempts FROM memory_aux_jobs WHERE job_id=$1`, j.ID).Scan(&state, &reason, &attempts); e != nil || state != "pending" || reason != "local_backend_unavailable" || attempts != 0 {
		t.Fatal(state, reason, attempts, e)
	}
	time.Sleep(time.Until(q.Job.Deadline) + 20*time.Millisecond)
	if e := w.Jobs.PollModelPolicies(ctx, func(string) bool { return true }); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT state,reason FROM memory_model_policies WHERE policy_id=$1`, q.ID).Scan(&state, &reason); e != nil || state != "revoked" || reason != "deadline" {
		t.Fatal(state, reason, e)
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
		t.Fatal("hidden cloud fallback")
	}
}

func TestModelWorkerShutdownNormalizedTransportErrorIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	j := schedule(t, w, ctx, policyRequest(src))
	worker, cancel := context.WithCancel(ctx)
	p := w.Provider.(*modelFixtureProvider)
	p.callback = cancel
	p.terminalError = transport.NormalizeError(context.Canceled, p.Name())
	if e := w.execute(worker, j, QuoteProbe{}); e == nil {
		t.Fatal("shutdown not observed")
	}
	var state, reason string
	var attempts int
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT status,error_code,attempts FROM memory_aux_jobs WHERE job_id=$1`, j.ID).Scan(&state, &reason, &attempts); e != nil || state != "pending" || reason != "worker_stopped" || attempts != 1 {
		t.Fatal(state, reason, attempts, e)
	}
	p.callback, p.terminalError = nil, nil
	if _, e := w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	if worked, e := w.ProcessOne(ctx); e != nil || !worked {
		t.Fatal(worked, e)
	}
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&attempts); e != nil || attempts != 2 {
		t.Fatal("shutdown reservation was lost", attempts, e)
	}
}

func TestModelMigrationRollbackRetainsHistoryIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	down, e := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "071_memory_model_jobs.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.Jobs.pool.Exec(ctx, string(down)); e != nil {
		t.Fatal("unused migration rollback", e)
	}
	up, e := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "071_memory_model_jobs.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.Jobs.pool.Exec(ctx, string(up)); e != nil {
		t.Fatal("reapply migration", e)
	}
	if _, e = w.Jobs.CreateModelPolicy(ctx, policyRequest(src)); e != nil {
		t.Fatal(e)
	}
	if _, e = w.Jobs.pool.Exec(ctx, string(down)); e == nil {
		t.Fatal("history was discarded by rollback")
	}
	var count int
	if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_policies`).Scan(&count); e != nil || count != 1 {
		t.Fatal("history not preserved", count, e)
	}
}
