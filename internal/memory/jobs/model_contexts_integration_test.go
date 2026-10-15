package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
)

func crossContextFixture(t *testing.T, turns int64) (*ModelWorker, context.Context, ModelPolicyRequest, []catalog.Source) {
	t.Helper()
	w, ctx, _ := modelFixture(t)
	w.Jobs.budgets.Policy.ModelMillis = 10000
	seedModelMandate(t, w, ctx, "cloud_allowed", turns)
	_, err := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s2','owner','web');
 UPDATE tasks SET session_key='s2' WHERE task_id='t2';
 INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)
 VALUES('r','s','t',1,'i','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes'),
 ('r2','s2','t2',1,'i2','completed','mode_1','completed','fixture',clock_timestamp()-INTERVAL '2 minutes',clock_timestamp()-INTERVAL '2 minutes')`)
	if err != nil {
		t.Fatal(err)
	}
	sources := []catalog.Source{}
	for _, session := range []string{"s", "s2"} {
		s, err := w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "context:" + session, Kind: "document", ScopeType: "session", ScopeID: session, Title: "observation", Body: "Гипотеза не подтверждена; исключение сохраняется."})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, s)
	}
	w.Provider.(*modelFixtureProvider).source = sources[0]
	q := policyRequest(sources[0])
	q.Handler = ConsolidationHandler
	q.Job.Allowance.ActiveMillis = 40000
	q.Contexts = []ModelContextRef{{SessionKey: "s2", OriginalRunID: "r2", TaskID: "t2"}, {SessionKey: "s", OriginalRunID: "r", TaskID: "t"}}
	q.Job.Sources = append(q.Job.Sources, request(sources[1]).Sources...)
	return w, ctx, q, sources
}

func TestModelCrossContextFullAuthorityAndSharedAtomicBudgetIntegration(t *testing.T) {
	w, ctx, q, sources := crossContextFixture(t, 1)
	j := schedule(t, w, ctx, q)
	if len(j.Contexts) != 2 || j.TaskID != "" || j.OriginalRunID != "" || j.Contexts[0].TaskID != "t" {
		t.Fatal("context snapshot lost", j)
	}
	_, legacy, _ := canonical(j.Sources)
	if legacy == j.Fingerprint {
		t.Fatal("context missing from fingerprint")
	}
	if err := w.Jobs.checkModelSupervisor(ctx, j); err != nil {
		t.Fatal("current thin proof", err)
	}
	if _, err := w.Jobs.catalog.Get(w.Jobs.Context(context.Background(), j), sources[1].ID); err != nil {
		t.Fatal("cross-context current source denied", err)
	}
	if err := w.execute(ctx, j, QuoteProbe{}); err != nil {
		t.Fatal(err)
	}
	var roots, mandates, days, reservations, incorrect int
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='task'),count(*) FILTER(WHERE kind='mandate'),count(*) FILTER(WHERE kind='owner_day'),count(*) FILTER(WHERE (used_json->>'model_turns')::int<>1) FROM execution_budget_scopes`).Scan(&roots, &mandates, &days, &incorrect); err != nil || roots != 2 || mandates != 1 || days != 1 || incorrect != 0 {
		t.Fatal("quote not charged to exact deduplicated scopes", roots, mandates, days, incorrect, err)
	}
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatal(reservations, err)
	}
	var before, after []byte
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	q.ID += "-second"
	j2 := schedule(t, w, ctx, q)
	if _, _, err := w.Admit(ctx, j2); err == nil {
		t.Fatal("shared mandate replenished")
	}
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s`).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatal("failed admission partially charged or reset scopes", err)
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 1 {
		t.Fatal("unexpected HTTP")
	}
}

func mutateContext(t *testing.T, w *ModelWorker, ctx context.Context, j Job, src catalog.Source, change string) {
	t.Helper()
	var err error
	switch change {
	case "task_revision":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id='t2'`)
	case "constraints_revision":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id='t2'`)
	case "task_stop":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE tasks SET status='cancelled',revision=revision+1 WHERE task_id='t2'`)
	case "session_owner":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE sessions SET user_id='foreign' WHERE session_key='s2'`)
	case "run_task":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE runs SET task_id=NULL,task_revision=NULL WHERE run_id='r2'`)
	case "mixed_task":
		_, err = w.Jobs.pool.Exec(ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('other','s2','owner','unrelated private task')`)
	case "quiet":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE runs SET updated_at=clock_timestamp() WHERE run_id='r2'`)
	case "mandate_revoke":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id='mandate'`)
	case "privacy":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE action_mandates SET mandate_data=jsonb_set(mandate_data,'{privacy}','"local_only"') WHERE mandate_id='mandate'`)
	case "source_correction":
		_, err = w.Jobs.catalog.Correct(ctx, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Исправленное наблюдение."})
	case "source_delete":
		_, err = w.Jobs.catalog.Delete(ctx, src.ID, "delete", src.Revision)
	case "job_context":
		_, err = w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET context_manifest=jsonb_set(context_manifest,'{0,task_revision}','999') WHERE job_id=$1`, j.ID)
	case "policy_revoke":
		err = w.Jobs.ControlModel(ctx, j.PolicyID, "revoked")
	case "worker_disable":
		err = w.Jobs.SetModelControl(ctx, "disabled")
	default:
		t.Fatal(change)
	}
	if err != nil {
		t.Fatal(change, err)
	}
}

func TestModelCrossContextCurrentMutationFencesHTTPAndLatePublicationIntegration(t *testing.T) {
	for _, change := range []string{"task_revision", "constraints_revision", "task_stop", "session_owner", "run_task", "mixed_task", "quiet", "mandate_revoke", "privacy", "source_correction", "source_delete", "job_context", "policy_revoke", "worker_disable"} {
		for _, stage := range []string{"before_http", "during_http"} {
			t.Run(change+"/"+stage, func(t *testing.T) {
				w, ctx, q, sources := crossContextFixture(t, 4)
				j := schedule(t, w, ctx, q)
				p := w.Provider.(*modelFixtureProvider)
				mutation := func() { mutateContext(t, w, ctx, j, sources[1], change) }
				if stage == "before_http" {
					p.beforeDispatch = mutation
				} else {
					p.callback = mutation
				}
				if err := w.execute(ctx, j, QuoteProbe{}); err == nil {
					t.Fatal("changed cross-context authority published")
				}
				wantCalls := int32(0)
				if stage == "during_http" {
					wantCalls = 1
				}
				if p.calls.Load() != wantCalls {
					t.Fatal("unexpected dispatch", p.calls.Load(), wantCalls)
				}
				var published, attempts, reserved, known int
				if err := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='published'),count(*),count(*) FILTER(WHERE usage IS NOT NULL),(SELECT count(*) FROM execution_budget_reservations) FROM memory_model_attempts`).Scan(&published, &attempts, &known, &reserved); err != nil || published != 0 || attempts != 1 || reserved != 1 || known != int(wantCalls) {
					t.Fatal("late failure erased or changed spending", published, attempts, known, reserved, err)
				}
			})
		}
	}
}

func TestModelCrossContextCannotExtendOtherConsumersOrForgePinnedJobIntegration(t *testing.T) {
	w, ctx, q, _ := crossContextFixture(t, 4)
	for _, change := range []string{"other_handler", "top_task", "top_run", "duplicate_session", "foreign_session", "wrong_run", "missing_task", "local_only", "too_many"} {
		t.Run(change, func(t *testing.T) {
			raw, _ := json.Marshal(q)
			var invalid ModelPolicyRequest
			_ = json.Unmarshal(raw, &invalid)
			switch change {
			case "other_handler":
				invalid.Handler = "source-quote-v1"
			case "top_task":
				invalid.Job.TaskID = "t"
			case "top_run":
				invalid.Job.OriginalRunID = "r"
			case "duplicate_session":
				invalid.Contexts[1] = invalid.Contexts[0]
			case "foreign_session":
				invalid.Contexts[0].SessionKey = "unknown"
			case "wrong_run":
				invalid.Contexts[0].OriginalRunID = "r"
			case "missing_task":
				invalid.Contexts[0].TaskID = ""
			case "local_only":
				invalid.Privacy = "local_only"
			case "too_many":
				for range 7 {
					invalid.Contexts = append(invalid.Contexts, ModelContextRef{SessionKey: "extra", OriginalRunID: "extra"})
				}
			}
			if _, err := w.Jobs.CreateModelPolicy(ctx, invalid); err == nil {
				t.Fatal("invalid context extension allowed")
			}
		})
	}
	j := schedule(t, w, ctx, q)
	for _, change := range []string{"omit", "mutate", "recompute"} {
		forged := copyJob(j)
		if change == "omit" {
			forged.Contexts = nil
		} else {
			forged.Contexts[0].TaskRevision++
		}
		if change == "recompute" {
			forged.Fingerprint, _ = modelFingerprint(forged.Sources, forged.Contexts)
		}
		if _, err := w.Jobs.catalog.Get(w.Jobs.Context(context.Background(), forged), j.Sources[0].ID); !errors.Is(err, ErrClaim) {
			t.Fatal("forged context got source", change, err)
		}
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
		t.Fatal("invalid context invoked HTTP")
	}
}

type stagedContextQuote struct{ QuoteProbe }

func (stagedContextQuote) StagedModelResult() bool { return true }

func TestModelStagedResultAndAtomicOwnerPolicySnapshotIntegration(t *testing.T) {
	w, ctx, q, sources := crossContextFixture(t, 4)
	tx, err := w.Jobs.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = w.Jobs.PrepareConsolidationContextsTx(ctx, tx, q); err != nil {
		t.Fatal(err)
	}
	snapshot, err := w.Jobs.catalog.CreateDerivedTx(ctx, tx, q.Job.Sources, "memory_job_create", "Prepared evidence", "Complete exact source set.", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared := q
	prepared.Job.Sources = request(snapshot).Sources
	if _, err = w.Jobs.CreateModelPolicyTx(ctx, tx, prepared); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var leaked int
	if err = w.Jobs.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_model_policies)+(SELECT count(*) FROM memory_sources WHERE source_id=$1)`, snapshot.ID).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatal("rolled back prepare leaked input or policy", leaked, err)
	}
	j := schedule(t, w, ctx, q)
	if err = w.execute(ctx, j, stagedContextQuote{}); err != nil {
		t.Fatal(err)
	}
	var resultID string
	if err = w.Jobs.pool.QueryRow(ctx, `SELECT result_source_id FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&resultID); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Jobs.catalog.Get(ctx, resultID); !errors.Is(err, catalog.ErrDenied) {
		t.Fatal("unreviewed staged result readable", err)
	}
	inspected, err := w.Jobs.catalog.Inspect(ctx, resultID)
	if err != nil || inspected.Source.Status != "quarantined" || inspected.Source.Body == "" {
		t.Fatal("owner cannot review exact staged data", err)
	}
	for _, s := range sources {
		if _, err = w.Jobs.catalog.Get(ctx, s.ID); err != nil {
			t.Fatal("staging damaged original", err)
		}
	}
}

func TestModelCrossContextDepletedRootRollsBackEveryChargeIntegration(t *testing.T) {
	w, ctx, q, _ := crossContextFixture(t, 4)
	j := schedule(t, w, ctx, q)
	var created time.Time
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT created_at FROM tasks WHERE task_id='t2'`).Scan(&created); err != nil {
		t.Fatal(err)
	}
	tx, err := w.Jobs.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	scope, err := w.Jobs.budgets.Ledger.EnsureTx(ctx, tx, budget.ScopeSpec{Kind: "task", OwnerID: "owner", SubjectID: "t2", Limits: w.Jobs.budgets.Policy.Task, Deadline: created.Add(time.Duration(w.Jobs.budgets.Policy.TaskLifetimeMillis) * time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE execution_budget_scopes SET used_json=jsonb_set(used_json,'{model_turns}',to_jsonb($2::bigint)) WHERE scope_id=$1`, scope.ID, w.Jobs.budgets.Policy.Task.ModelTurns); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var before, after []byte
	if err = w.Jobs.pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, _, err = w.Admit(ctx, j); err == nil {
		t.Fatal("one depleted root did not block all contexts")
	}
	if err = w.Jobs.pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s`).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatal("one depleted root left a partial charge", err)
	}
	var attempts, reservations int
	if err = w.Jobs.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_model_attempts),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&attempts, &reservations); err != nil || attempts != 0 || reservations != 0 {
		t.Fatal(attempts, reservations, err)
	}
}

func TestModelCrossContextEarliestIndependentAssignmentDeadlineIntegration(t *testing.T) {
	w, ctx, q, _ := crossContextFixture(t, 4)
	w.Jobs.budgets.Policy.ModelMillis = 60000
	q.Job.Allowance.ActiveMillis = 240000
	var raw []byte
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT mandate_data FROM action_mandates WHERE mandate_id='mandate'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var m actionauth.Mandate
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Budget.ActiveMillis = 240000
	if _, err := w.Jobs.pool.Exec(ctx, `UPDATE action_mandates SET mandate_data=$1 WHERE mandate_id='mandate'`, jsonBytes(m)); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(30 * time.Second).UTC().Truncate(time.Microsecond)
	m.ID, m.GoalID, m.ExpiresAt, m.BudgetDeadline = "mandate2", "goal2", expiry, expiry
	g := actionauth.GoalPolicy{ID: "goal2", OwnerID: "owner", Kind: "objective", Status: "active", Revision: 1, Purpose: "memory", ExpiresAt: expiry}
	if _, err := w.Jobs.pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal2','owner','objective','active',1,$1,$2,'{}')`, expiry, jsonBytes(g)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Jobs.pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate2','owner','',1,$1,$2,'owner_goal','goal2',1,'active')`, jsonBytes(m), expiry); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Jobs.pool.Exec(ctx, `UPDATE task_authority_bindings SET mandate_id='mandate2',goal_id='goal2' WHERE task_id='t2'`); err != nil {
		t.Fatal(err)
	}
	j := schedule(t, w, ctx, q)
	admitted, _, err := w.Admit(ctx, j)
	if err != nil || admitted.Deadline.After(expiry) || !admitted.Deadline.Equal(expiry) {
		t.Fatal("second context deadline lost", admitted.Deadline, expiry, err)
	}
}
