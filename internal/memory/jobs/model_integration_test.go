package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
)

type modelFixtureProvider struct {
	transport.ModelProvider
	calls          atomic.Int32
	source         catalog.Source
	callback       func()
	beforeDispatch func()
	overrun        bool
	unavailable    bool
	unknown        bool
	terminalError  error
}

func (*modelFixtureProvider) Name() string                    { return "antigravity" }
func (*modelFixtureProvider) InferenceLimits() (int64, int64) { return 128, 64 }
func (*modelFixtureProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *modelFixtureProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p.beforeDispatch != nil {
		p.beforeDispatch()
	}
	if e := transport.CheckInferenceAuthority(ctx); e != nil {
		return nil, e
	}
	if e := transport.CheckInferenceAuthority(ctx); e != nil {
		return nil, e
	}
	if e := transport.CheckInferenceDispatch(ctx); e != nil {
		return nil, e
	}
	p.calls.Add(1)
	if p.callback != nil {
		p.callback()
	}
	if p.terminalError != nil {
		return nil, p.terminalError
	}
	if p.unavailable {
		return nil, &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Retryable: true}
	}
	var u *transport.Usage
	if !p.unknown {
		u = &transport.Usage{InputTokens: 20, OutputTokens: 12, TotalTokens: 32}
		if p.overrun {
			u.OutputTokens = 80
			u.TotalTokens = 100
		}
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(jsonBytes(map[string]string{"source_id": p.source.ID, "quote": p.source.Body})), Usage: u})
	close(ch)
	return ch, nil
}
func modelFixture(t *testing.T) (*ModelWorker, context.Context, catalog.Source) {
	s, _, ctx, src := fixture(t)
	s.budgets.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	s.budgets.Policy.ModelMillis = 1000
	if e := s.SetModelControl(ctx, "enabled"); e != nil {
		t.Fatal(e)
	}
	return &ModelWorker{Jobs: s, Provider: &modelFixtureProvider{source: src}, Model: "fixture", Handlers: map[string]ModelHandler{"source-quote-v1": QuoteProbe{}}}, ctx, src
}
func policyRequest(src catalog.Source) ModelPolicyRequest {
	r := request(src)
	r.Allowance = budget.Units{ModelTurns: 4, Tokens: 10000, ActiveMillis: 10000, ProviderInputBytes: 4 * 8388608}
	return ModelPolicyRequest{ID: "policy-model-test", Handler: "source-quote-v1", Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 4, Privacy: "cloud_allowed", Job: r}
}
func schedule(t *testing.T, w *ModelWorker, ctx context.Context, q ModelPolicyRequest) Job {
	t.Helper()
	if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.PollModelPolicies(ctx, func(string) bool { return true }); e != nil {
		t.Fatal(e)
	}
	j, ok, e := w.Jobs.ClaimKind(ctx, "model")
	if e != nil || !ok {
		v, _ := w.Jobs.ModelView(ctx)
		for _, r := range v {
			t.Log(string(r))
		}
		t.Fatal(ok, e)
	}
	return j
}
func TestModelFailureDiagnosticExcludesProviderMaterialIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	j := schedule(t, w, ctx, policyRequest(src))
	w.Provider.(*modelFixtureProvider).terminalError = &transport.Error{Type: transport.ErrorTypeProviderProtocolError, HTTPStatus: 400, Message: "PRIVATE_PROVIDER_BODY", ProviderCode: "PRIVATE_ACCOUNT", ProviderDetails: map[string]any{"token": "PRIVATE_MATERIAL"}}
	if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
		t.Fatal("provider failure not observed")
	}
	var reason string
	if e := w.Jobs.pool.QueryRow(ctx, `SELECT reason FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&reason); e != nil || reason != "provider_protocol_error_http_400" {
		t.Fatal(reason, e)
	}
	view, e := w.Jobs.ModelView(ctx)
	if e != nil || strings.Contains(string(jsonBytes(view)), "PRIVATE_") {
		t.Fatal("private provider material leaked into observations")
	}
}

func TestModelCredentialSourceNeverReachesProviderIntegration(t *testing.T) {
	w, ctx, _ := modelFixture(t)
	src, e := w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "private-fixture", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "source", Body: "password=ONLY_SYNTHETIC_PRIVATE_FIXTURE"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.Jobs.CreateModelPolicy(ctx, policyRequest(src)); e == nil {
		t.Fatal("credential source admitted")
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
		t.Fatal("credential source reached provider")
	}
}

func TestModelCompletedRunAndTaskPolicyIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('t','s','owner','finished task','completed'); INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('r','s','t',1,'i','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`)
	if e != nil {
		t.Fatal(e)
	}
	q := policyRequest(src)
	q.Job.TaskID = "t"
	q.Job.OriginalRunID = "r"
	q.Trigger = "session_completed"
	j := schedule(t, w, ctx, q)
	if e = w.execute(ctx, j, QuoteProbe{}); e != nil {
		t.Fatal(e)
	}
	var state, result, taskState string
	var usage []byte
	e = w.Jobs.pool.QueryRow(ctx, `SELECT j.status,a.result_source_id,a.usage,t.status FROM memory_aux_jobs j JOIN memory_model_attempts a USING(job_id) JOIN tasks t ON t.task_id=j.task_id WHERE j.job_id=$1`, j.ID).Scan(&state, &result, &usage, &taskState)
	if e != nil || state != "done" || result == "" || len(usage) == 0 || taskState != "completed" {
		t.Fatal(state, result, string(usage), taskState, e)
	}
	if _, e = w.Jobs.catalog.Get(ctx, result); e != nil {
		t.Fatal(e)
	}
	t.Log("completed task remained completed; no lease; provider attempt, usage, catalog DATA and job done committed")
}
func TestModelCrashRecoveryAndLostPublicationACKIntegration(t *testing.T) {
	for _, phase := range []string{"before_dispatch", "after_dispatch", "before_commit"} {
		t.Run(phase, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			// This tests crash fencing and ACK recovery, not latency. Two 5s
			// quotes fit the existing shared 10s allowance under race/CPU load.
			w.Jobs.budgets.Policy.ModelMillis = 5000
			j := schedule(t, w, ctx, policyRequest(src))
			a, _, e := w.Admit(ctx, j)
			if e != nil {
				t.Fatal(e)
			}
			if phase != "before_dispatch" {
				if e = w.Jobs.dispatchModel(ctx, j, a); e != nil {
					t.Fatal(e)
				}
			}
			if phase == "before_commit" {
				if e = w.Jobs.observeModel(ctx, a, nil, "original not published"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET claim_until=clock_timestamp()-INTERVAL '1 second' WHERE job_id=$1`, j.ID); e != nil {
				t.Fatal(e)
			}
			replacement, ok, e := w.Jobs.ClaimKind(ctx, "model")
			if e != nil || !ok {
				t.Fatal(ok, e)
			}
			if _, e = w.Jobs.publishModel(ctx, j, a, "late old output"); e == nil {
				t.Fatal("expired claim published")
			}
			if e = w.execute(ctx, replacement, QuoteProbe{}); e != nil {
				t.Fatal(e)
			}
			var attempts, published, unknown int
			if e = w.Jobs.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state='published'),count(*) FILTER(WHERE usage IS NULL) FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&attempts, &published, &unknown); e != nil || attempts != 2 || published != 1 || unknown != 1 {
				t.Fatal(attempts, published, unknown, e)
			}
			scope, e := w.Jobs.budgets.Ledger.Get(ctx, budget.ScopeID("auxiliary", "owner", j.ID))
			if e != nil || scope.Used.ModelTurns != 2 {
				t.Fatal(scope, e)
			}
			var op, result string
			if e = w.Jobs.pool.QueryRow(ctx, `SELECT operation_id,result_source_id FROM memory_model_attempts WHERE job_id=$1 AND state='published'`, j.ID).Scan(&op, &result); e != nil {
				t.Fatal(e)
			}
			got, e := w.Jobs.publishModel(ctx, replacement, ModelAttempt{OperationID: op}, "ack retry")
			if e != nil || got != result {
				t.Fatal(got, e)
			}
			if e = w.Jobs.scheduleModelPolicy(ctx, j.PolicyID); e != nil {
				t.Fatal(e)
			}
			var count int
			_ = w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_aux_jobs WHERE policy_id=$1`, j.PolicyID).Scan(&count)
			if count != 1 {
				t.Fatal("duplicate event", count)
			}
		})
	}
}
func TestModelConcurrentWorkersAndBudgetIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	q := policyRequest(src)
	q.Job.Allowance.ModelTurns = 1
	if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := w.ProcessOne(ctx)
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if w.Provider.(*modelFixtureProvider).calls.Load() != 1 {
		t.Fatal("competing dispatch")
	}
	updated, e := w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "message", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: src.Revision, Title: "source", Body: "new version"})
	if e != nil {
		t.Fatal(e)
	}
	w.Provider.(*modelFixtureProvider).source = updated
	if e = w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
		t.Fatal(e)
	}
	_, e = w.ProcessOne(ctx)
	var exhausted *budget.ExhaustedError
	if !errors.As(e, &exhausted) {
		t.Fatal("aggregate policy budget", e)
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 1 {
		t.Fatal("budget exhausted dispatch")
	}
}
func TestModelRevocationDuringCallIntegration(t *testing.T) {
	for _, change := range []string{"correct", "delete", "revoke", "cancel", "pause", "disable", "task_amend"} {
		t.Run(change, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			q := policyRequest(src)
			if change == "task_amend" {
				_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('s','owner','web'); INSERT INTO tasks(task_id,session_key,owner_id,goal,status) VALUES('t','s','owner','finished','completed')`)
				if e != nil {
					t.Fatal(e)
				}
				q.Job.TaskID = "t"
			}
			j := schedule(t, w, ctx, q)
			w.Provider.(*modelFixtureProvider).callback = func() {
				var e error
				switch change {
				case "correct":
					_, e = w.Jobs.catalog.Put(ctx, catalog.PutSource{ExternalKey: "message", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: src.Revision, Title: "source", Body: "corrected"})
				case "delete":
					_, e = w.Jobs.catalog.Delete(ctx, src.ID, "delete", src.Revision)
				case "revoke":
					e = w.Jobs.ControlModel(ctx, q.ID, "revoked")
				case "pause":
					e = w.Jobs.ControlModel(ctx, q.ID, "paused")
				case "disable":
					e = w.Jobs.SetModelControl(ctx, "disabled")
				case "cancel":
					e = w.Jobs.CancelModelJob(ctx, j.ID)
				case "task_amend":
					_, e = w.Jobs.pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id='t'`)
				}
				if e != nil {
					t.Fatal(e)
				}
			}
			if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
				t.Fatal("late publication allowed")
			}
			var count int
			_ = w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts WHERE job_id=$1 AND state='published'`, j.ID).Scan(&count)
			if count != 0 {
				t.Fatal("late result", count)
			}
			var usage []byte
			_ = w.Jobs.pool.QueryRow(ctx, `SELECT usage FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&usage)
			if len(usage) == 0 {
				t.Fatal("observed spend lost")
			}
		})
	}
}
func TestModelLocalOnlyUnavailableAndRetryBoundIntegration(t *testing.T) {
	t.Run("local_only", func(t *testing.T) {
		w, ctx, src := modelFixture(t)
		q := policyRequest(src)
		q.Privacy = "local_only"
		j := schedule(t, w, ctx, q)
		if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
			t.Fatal("cloud release")
		}
		if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
			t.Fatal("hidden fallback")
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		w, ctx, src := modelFixture(t)
		w.Provider.(*modelFixtureProvider).unavailable = true
		j := schedule(t, w, ctx, policyRequest(src))
		for i := 0; i < 4; i++ {
			if e := w.execute(ctx, j, QuoteProbe{}); e == nil {
				t.Fatal("unavailable succeeded")
			}
			_, _ = w.Jobs.pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id=$1`, j.ID)
			if i < 3 {
				var ok bool
				var e error
				j, ok, e = w.Jobs.ClaimKind(ctx, "model")
				if e != nil || !ok {
					t.Fatal(ok, e)
				}
			}
		}
		var state string
		_ = w.Jobs.pool.QueryRow(ctx, `SELECT status FROM memory_aux_jobs WHERE job_id=$1`, j.ID).Scan(&state)
		if state != "failed" || w.Provider.(*modelFixtureProvider).calls.Load() != 4 {
			t.Fatal(state)
		}
		_, ok, e := w.Jobs.ClaimKind(ctx, "model")
		if e != nil || ok {
			t.Fatal("unbounded retries", ok, e)
		}
	})
	t.Run("unknown_usage", func(t *testing.T) {
		w, ctx, src := modelFixture(t)
		w.Provider.(*modelFixtureProvider).unknown = true
		j := schedule(t, w, ctx, policyRequest(src))
		if e := w.execute(ctx, j, QuoteProbe{}); e != nil {
			t.Fatal(e)
		}
		var null bool
		_ = w.Jobs.pool.QueryRow(ctx, `SELECT usage IS NULL FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&null)
		if !null {
			t.Fatal("missing usage became zero")
		}
	})
}
func TestModelPriorityPauseDeadlineAndConsumerGateIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	q := policyRequest(src)
	if _, e := w.Jobs.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.PollModelPolicies(ctx, func(string) bool { return false }); e != nil {
		t.Fatal(e)
	}
	var count int
	_ = w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM memory_aux_jobs WHERE kind='model'`).Scan(&count)
	if count != 0 {
		t.Fatal("ungated consumer")
	}
	if e := w.Jobs.SetModelControl(ctx, "paused"); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
		t.Fatal(e)
	}
	var reason string
	_ = w.Jobs.pool.QueryRow(ctx, `SELECT reason FROM memory_model_policies WHERE policy_id=$1`, q.ID).Scan(&reason)
	if reason != "worker_paused" {
		t.Fatal(reason)
	}
	if e := w.Jobs.SetModelControl(ctx, "enabled"); e != nil {
		t.Fatal(e)
	}
	if e := w.Jobs.scheduleModelPolicy(ctx, q.ID); e != nil {
		t.Fatal(e)
	}
	// Interactive work has already obtained Session Service ownership.
	_, e := w.Jobs.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('interactive','owner','web'); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('interactive','interactive-run','lease',1,clock_timestamp()+INTERVAL '1 minute')`)
	if e != nil {
		t.Fatal(e)
	}
	worked, e := w.ProcessOne(ctx)
	if e != nil || worked {
		t.Fatal("interactive priority", worked, e)
	}
	if w.Provider.(*modelFixtureProvider).calls.Load() != 0 {
		t.Fatal("interactive overlap")
	}
}

func TestQuoteProbeRejectsUnverifiableModelOutput(t *testing.T) {
	src := catalog.Source{ID: "s", Body: "literal"}
	for _, raw := range []string{`{"source_id":"s","quote":"invented"}`, `{"source_id":"s","quote":"literal","accepted":true}`, `{"source_id":"s","quote":"literal","quote":"literal"}`, `{"source_id":"other","quote":"literal"}`} {
		if _, e := (QuoteProbe{}).Validate([]catalog.Source{src}, raw); e == nil {
			t.Fatal(raw)
		}
	}
}
