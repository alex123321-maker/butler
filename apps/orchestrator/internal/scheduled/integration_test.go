package scheduled

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/jackc/pgx/v5"
)

type fixture struct {
	s   *Store
	p   Principal
	ctx context.Context
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('schedule-chat','schedule-owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('schedule-source','schedule-chat','source','completed','completed','mode_1','test',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "schedule-deletions.log"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.Models["test:test"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	s := &Store{Pool: pool, Tasks: task.NewStore(pool), Inputs: durableinput.NewStore(pool), Budgets: budget.NewController(pool, policy), Allowance: budget.ModelAllowance{Provider: "test", Model: "test", InputTokens: 1024, OutputTokens: 8192}, SourceGuard: sources.CheckRunSourcesTx}
	if err := s.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	return &fixture{s: s, p: Principal{OwnerID: "schedule-owner", SessionKey: "schedule-chat", RunID: "schedule-source", ExecutionPolicy: "normal", Constraints: []string{}, Approved: true}, ctx: ctx}
}
func spec(mode string) schedulecontrol.Spec {
	return schedulecontrol.Spec{Title: "Check later", Mode: mode, Instruction: "Проверь состояние проекта", CompletionCriteria: []string{"Evidence recorded"}, Timing: schedulecontrol.Timing{Kind: "once", TimeZone: "Asia/Tomsk", AfterSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}, ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond), MaxOccurrences: 1}
}
func (f *fixture) apply(t *testing.T, tool, effect string, a schedulecontrol.Args) (schedulecontrol.Receipt, error) {
	t.Helper()
	tx, err := f.s.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	raw, _ := json.Marshal(a)
	hash, _ := actionauth.ArgumentsHash(string(raw))
	r, err := f.s.ApplyTx(f.ctx, tx, f.p, tool, effect, hash, a)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(f.ctx)
}
func (f *fixture) create(t *testing.T, sp schedulecontrol.Spec) schedulecontrol.View {
	t.Helper()
	r, err := f.apply(t, "schedule.create", "create", schedulecontrol.Args{Job: &sp})
	if err != nil {
		t.Fatal(err)
	}
	return *r.Schedule
}
func (f *fixture) due(t *testing.T, id string) {
	t.Helper()
	var consumed int
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM scheduled_occurrences WHERE schedule_id=$1`, id).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Duration(consumed+1) * time.Second).UTC().Truncate(time.Microsecond)
	if _, err := f.s.Pool.Exec(f.ctx, `UPDATE scheduled_jobs SET next_at=$2,spec_json=CASE WHEN spec_json #>> '{timing,kind}'='once' THEN jsonb_set(spec_json,'{timing,at}',to_jsonb($2::timestamptz)) ELSE spec_json END WHERE schedule_id=$1`, id, past); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) view(t *testing.T, id string) schedulecontrol.View {
	t.Helper()
	tx, err := f.s.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	j, err := scanJob(tx.QueryRow(f.ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE schedule_id=$1`, id))
	if err != nil {
		t.Fatal(err)
	}
	j.History, err = f.s.historyTx(f.ctx, tx, id)
	if err != nil {
		t.Fatal(err)
	}
	return j.View
}

func TestScheduleRestartIdempotencyAndConcurrentAdmissionIntegration(t *testing.T) {
	f := setup(t)
	sp := spec("agent_task")
	v := f.create(t, sp)
	repeated, err := f.apply(t, "schedule.create", "create", schedulecontrol.Args{Job: &sp})
	if err != nil || repeated.Schedule.ID != v.ID {
		t.Fatal("lost receipt created duplicate", err)
	}
	changed := sp
	changed.Instruction = "Other work"
	if _, err = f.apply(t, "schedule.create", "create", schedulecontrol.Args{Job: &changed}); err == nil {
		t.Fatal("same effect accepted changed arguments")
	}
	f.due(t, v.ID)
	restored := *f.s
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- restored.advance(f.ctx, v.ID) }()
	}
	wg.Wait()
	close(errs)
	for err = range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	actual := f.view(t, v.ID)
	if actual.State != "finished" || actual.OccurrenceCount != 1 || len(actual.History) != 1 || actual.History[0].TaskID == "" || actual.History[0].InputID == "" {
		t.Fatalf("actual admission %+v", actual)
	}
	var tasks, inputs int
	if err = f.s.Pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM accepted_inputs)`).Scan(&tasks, &inputs); err != nil || tasks != 1 || inputs != 1 {
		t.Fatal("duplicate task/input", tasks, inputs, err)
	}
}
func TestScheduleControlsAndFiniteAllowanceIntegration(t *testing.T) {
	f := setup(t)
	sp := spec("agent_task")
	sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
	sp.MaxOccurrences = 3
	v := f.create(t, sp)
	sp.Paused = true
	r, err := f.apply(t, "schedule.update", "pause", schedulecontrol.Args{Job: &sp, ScheduleID: v.ID, ExpectedRevision: 1})
	if err != nil || r.Schedule.State != "paused" {
		t.Fatal("pause", err)
	}
	f.due(t, v.ID)
	if err = f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.view(t, v.ID).History) != 0 {
		t.Fatal("paused work fired")
	}
	sp.Paused = false
	r, err = f.apply(t, "schedule.update", "resume", schedulecontrol.Args{Job: &sp, ScheduleID: v.ID, ExpectedRevision: 2})
	if err != nil || r.Schedule.State != "active" {
		t.Fatal("resume", err)
	}
	if _, err = f.apply(t, "schedule.cancel", "stale", schedulecontrol.Args{ScheduleID: v.ID, ExpectedRevision: 2}); err == nil {
		t.Fatal("stale revision cancelled schedule")
	}
	extended := sp
	extended.MaxOccurrences = 4
	if _, err = f.apply(t, "schedule.update", "extend", schedulecontrol.Args{Job: &extended, ScheduleID: v.ID, ExpectedRevision: 3}); err == nil {
		t.Fatal("allowance reset")
	}
	f.due(t, v.ID)
	if err = f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	actual := f.view(t, v.ID)
	taskID := actual.History[0].TaskID
	if _, err = f.apply(t, "schedule.cancel", "cancel", schedulecontrol.Args{ScheduleID: v.ID, ExpectedRevision: 3}); err != nil {
		t.Fatal(err)
	}
	var inputState string
	if err = f.s.Pool.QueryRow(f.ctx, `SELECT status FROM accepted_inputs WHERE task_id=$1`, taskID).Scan(&inputState); err != nil || inputState != "cancelled" {
		t.Fatal("old input still blocks conversation", inputState, err)
	}
	tx, _ := f.s.Pool.Begin(f.ctx)
	defer tx.Rollback(f.ctx)
	if _, err = actionauth.CheckTaskAuthorityTx(f.ctx, tx, taskID, f.p.OwnerID, nil, time.Now()); err == nil {
		t.Fatal("queued task survived cancelled schedule")
	}
}

func TestSchedulePauseCancelsObsoleteInputAndResumeAdmitsFreshWorkIntegration(t *testing.T) {
	f := setup(t)
	sp := spec("agent_task")
	sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
	sp.MaxOccurrences = 4
	v := f.create(t, sp)
	f.due(t, v.ID)
	if err := f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='blocked'`); err != nil {
		t.Fatal(err)
	}
	sp.Paused = true
	if _, err := f.apply(t, "schedule.update", "pause-old-input", schedulecontrol.Args{Job: &sp, ScheduleID: v.ID, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	sp.Paused = false
	if _, err := f.apply(t, "schedule.update", "resume-old-input", schedulecontrol.Args{Job: &sp, ScheduleID: v.ID, ExpectedRevision: 2}); err != nil {
		t.Fatal(err)
	}
	f.due(t, v.ID)
	if err := f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	var cancelled, queued int
	if err := f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FILTER(WHERE status='cancelled'),count(*) FILTER(WHERE status='queued') FROM accepted_inputs`).Scan(&cancelled, &queued); err != nil || cancelled != 1 || queued != 1 {
		t.Fatal("obsolete input starved fresh revision", cancelled, queued, err)
	}
}
func TestScheduleRepeatUsesAttemptStateAndSkipsOverlapIntegration(t *testing.T) {
	f := setup(t)
	sp := spec("agent_task")
	sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
	sp.MaxOccurrences = 3
	v := f.create(t, sp)
	for i := 0; i < 3; i++ {
		f.due(t, v.ID)
		if err := f.s.Sweep(f.ctx); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, err := f.s.Pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='completed'`); err != nil {
				t.Fatal(err)
			}
		}
	}
	actual := f.view(t, v.ID)
	if actual.OccurrenceCount != 3 || actual.State != "finished" || len(actual.History) != 3 {
		t.Fatalf("repeat %+v", actual)
	}
	var admitted, skipped int
	for _, o := range actual.History {
		if o.State == "admitted" {
			admitted++
		}
		if o.Reason == "previous_occurrence_active" {
			skipped++
		}
	}
	if admitted != 2 || skipped != 1 {
		t.Fatalf("attempt completion should permit another unverified task: %d/%d", admitted, skipped)
	}
}
func TestScheduleReminderDeliveryAndUnknownAcknowledgementIntegration(t *testing.T) {
	f := setup(t)
	if _, err := f.s.Pool.Exec(f.ctx, `UPDATE sessions SET channel='telegram'`); err != nil {
		t.Fatal(err)
	}
	v := f.create(t, spec("reminder"))
	f.due(t, v.ID)
	if err := f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	outbox := ReminderStore{f.s}
	m, err := outbox.Claim(f.ctx, 30*time.Second)
	if err != nil || m == nil {
		t.Fatal("notice not durable", err)
	}
	if err = outbox.AuthorizeSend(f.ctx, *m); err != nil {
		t.Fatal(err)
	}
	if err = outbox.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "unknown", ErrorCode: "timeout"}); err != nil {
		t.Fatal(err)
	}
	if next, err := outbox.Claim(f.ctx, 30*time.Second); err != nil || next != nil {
		t.Fatal("uncertain delivery retried", err)
	}
	actual := f.view(t, v.ID)
	if actual.History[0].DeliveryState != "delivery_unknown" {
		t.Fatalf("lost unknown %+v", actual)
	}
	var n int
	if err = f.s.Pool.QueryRow(f.ctx, `SELECT count(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
		t.Fatal("reminder admitted inference", n, err)
	}
}
func TestScheduleDenialRecordsBlockedAfterSQLSourceExceptionIntegration(t *testing.T) {
	f := setup(t)
	v := f.create(t, spec("agent_task"))
	f.due(t, v.ID)
	f.s.SourceGuard = func(ctx context.Context, tx pgx.Tx, _ string) error { _, err := tx.Exec(ctx, `SELECT 1/0`); return err }
	if err := f.s.Sweep(f.ctx); err != nil {
		t.Fatal("source exception lost blocked state", err)
	}
	actual := f.view(t, v.ID)
	if actual.State != "blocked" || actual.Reason != "source_changed" || len(actual.History) != 1 || actual.History[0].TaskID != "" {
		t.Fatalf("blocked %+v", actual)
	}
}

func TestScheduleSourceControlsExpiryAndPayloadProjectionIntegration(t *testing.T) {
	for _, scenario := range []string{"paused_source", "amended_source", "expired", "source_projection", "no_consent"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			if scenario == "no_consent" {
				f.p.Approved = false
				sp := spec("agent_task")
				if _, err := f.apply(t, "schedule.create", "denied", schedulecontrol.Args{Job: &sp}); err == nil {
					t.Fatal("schedule minted its own consent")
				}
				return
			}
			source, err := f.s.Tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.p.SessionKey, OwnerID: f.p.OwnerID, Goal: "Original instruction", CompletionCriteria: []string{"Accepted schedule"}})
			if err != nil {
				t.Fatal(err)
			}
			f.p.SourceTaskID = source.TaskID
			f.p.SourceGoal = source.Goal
			f.p.Constraints = source.Constraints
			if _, err = f.s.Pool.Exec(f.ctx, `UPDATE runs SET task_id=$1,task_revision=$2 WHERE run_id='schedule-source'`, source.TaskID, source.Revision); err != nil {
				t.Fatal(err)
			}
			v := f.create(t, spec("agent_task"))
			f.due(t, v.ID)
			switch scenario {
			case "paused_source":
				_, err = f.s.Pool.Exec(f.ctx, `UPDATE tasks SET status='paused' WHERE task_id=$1`, source.TaskID)
			case "amended_source":
				_, err = f.s.Pool.Exec(f.ctx, `UPDATE tasks SET constraints_json='["New owner restriction"]'::jsonb WHERE task_id=$1`, source.TaskID)
			case "expired":
				_, err = f.s.Pool.Exec(f.ctx, `UPDATE scheduled_jobs SET expires_at=clock_timestamp()-interval '1 second' WHERE schedule_id=$1`, v.ID)
			case "source_projection":
				f.s.SourceGuard = func(ctx context.Context, tx pgx.Tx, _ string) error { _, e := tx.Exec(ctx, `SELECT 1/0`); return e }
				for _, operation := range []string{"schedule.get", "schedule.list"} {
					a := schedulecontrol.Args{ScheduleID: v.ID}
					if operation == "schedule.list" {
						a = schedulecontrol.Args{}
					}
					r, e := f.apply(t, operation, operation, a)
					if e != nil {
						t.Fatal(e)
					}
					views := r.Schedules
					if r.Schedule != nil {
						views = []schedulecontrol.View{*r.Schedule}
					}
					if len(views) != 1 || views[0].Instruction != "" || views[0].Reason != "source_unavailable" {
						t.Fatal("restricted instruction republished", views)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.Sweep(f.ctx); err != nil {
				t.Fatal(err)
			}
			actual := f.view(t, v.ID)
			if scenario == "expired" {
				if actual.State != "finished" || len(actual.History) != 0 {
					t.Fatal("expired job fired", actual)
				}
			} else if actual.State != "blocked" || actual.Reason != "source_changed" || actual.History[0].TaskID != "" {
				t.Fatal("source controls bypassed", actual)
			}
		})
	}
}

func TestScheduleMandateRevocationIsCurrentAtOccurrenceIntegration(t *testing.T) {
	f := setup(t)
	goals := goal.New(f.s.Pool, f.s.Tasks, nil)
	actor := func(delivery string) context.Context {
		return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: delivery})
	}
	now := time.Now().UTC()
	created, err := goals.PutGoal(actor("goal"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Maintain project", Purpose: "scheduled-work", ExpiresAt: now.Add(3 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	u := budget.DefaultPolicy().Mandate
	m := actionauth.Mandate{GoalID: created.SubjectID, GoalRevision: created.Revision, Status: "active", Purpose: "scheduled-work", Tools: []string{"schedule.create", "http.request"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "tool:schedule.create"}, {Origin: "https://example.invalid", PathPrefix: "/project"}}, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, CostMicros: u.CostMicros, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: now.Add(2 * time.Hour), AllowInitiative: true, Intervention: []string{"effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}
	mandate, err := goals.PutMandate(actor("mandate"), goal.MandateRequest{Mandate: m})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := goals.CheckAdmissionTx(f.ctx, tx, goal.AdmissionRequest{OwnerID: f.p.OwnerID, GoalID: created.SubjectID, MandateID: mandate.SubjectID, Purpose: "scheduled-work", ObservationRef: "schedule-owner-request", InitiativeReason: "Previously authorized scheduling"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.p.Authority = &a
	f.p.Approved = false
	sp := spec("agent_task")
	sp.ExpiresAt = now.Add(time.Hour)
	v := f.create(t, sp)
	m, err = actionauth.NewPostgresStore(f.s.Pool).GetMandate(f.ctx, mandate.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	f.p.Authority = nil
	inspected, err := f.apply(t, "schedule.get", "other-purpose", schedulecontrol.Args{ScheduleID: v.ID})
	if err != nil || inspected.Schedule.Instruction != "" || inspected.Schedule.Reason != "authority_scope_unavailable" {
		t.Fatal("another purpose exposed schedule payload", err)
	}
	f.p.Authority = &a
	m.Status = "paused"
	if _, err = goals.PutMandate(actor("pause"), goal.MandateRequest{Mandate: m, ExpectedRevision: mandate.Revision}); err != nil {
		t.Fatal(err)
	}
	f.due(t, v.ID)
	if err = f.s.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	actual := f.view(t, v.ID)
	if actual.State != "blocked" || actual.Reason != "authority_unavailable" || actual.History[0].TaskID != "" {
		t.Fatal("stale authority admitted scheduled task", actual)
	}
}
