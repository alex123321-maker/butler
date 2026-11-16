package compaction

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/acpqualification"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
)

func TestACPSummaryContinuityAndRestart(t *testing.T) {
	c := acpqualification.Open(t, "summary-v1", 2)
	b := budget.NewController(c.Pool, c.Policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	f := &consumerFixture{pool: c.Pool, cat: catalog.New(c.Pool, c.Manifest), ctx: owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "acp-summary-owner"})}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('session','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at)VALUES('run','session','input','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	f.js = jobs.New(c.Pool, f.cat, b, nil)
	if err := f.js.SetModelControl(f.ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	f.append(t, 12)
	f.worker = &jobs.ModelWorker{Jobs: f.js, Provider: c.Provider, Model: "gemini-3.8-flash", Handlers: map[string]jobs.ModelHandler{HandlerName: Consumer{ContextBudget: 4096}}}
	q := jobs.ModelPolicyRequest{ID: "acp-summary-v1", Handler: HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 2, Privacy: "cloud_allowed", AccountingMode: "opaque_subscription", Job: jobs.Request{OriginalRunID: "run", Allowance: c.Allowance(), Deadline: time.Now().Add(30 * time.Minute)}}
	if _, err := (&Policies{Catalog: f.cat, Jobs: f.js}).Create(f.ctx, PolicyRequest{SessionKey: "session", Policy: q}); err != nil {
		t.Fatal(err)
	}
	poll := func() {
		t.Helper()
		if _, err := f.pool.Exec(f.ctx, `UPDATE memory_model_policies SET next_event_at=clock_timestamp()-INTERVAL '1 second' WHERE policy_id=$1`, q.ID); err != nil {
			t.Fatal(err)
		}
		for scan := 0; scan < 20; scan++ {
			if err := f.js.PollModelPolicies(f.ctx, func(name string) bool { return name == HandlerName }, map[string]jobs.ModelScheduleHandler{HandlerName: Consumer{ContextBudget: 4096}}); err != nil {
				t.Fatal(err)
			}
			var ready bool
			if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM memory_aux_jobs WHERE policy_id=$1 AND status='pending')`, q.ID).Scan(&ready); err != nil {
				t.Fatal(err)
			}
			if ready {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatal("ordinary scanner did not admit summary")
	}
	poll()
	if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did {
		t.Fatal(did, err)
	}
	first, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
	if err != nil || first.ThroughSequence != 8 || !strings.Contains(first.Summary, "останови только отправку") {
		t.Fatal("first exact summary unavailable", err)
	}
	f.append(t, 9)
	f.worker = &jobs.ModelWorker{Jobs: jobs.New(f.pool, f.cat, b, nil), Provider: c.Provider, Model: "gemini-3.8-flash", Handlers: f.worker.Handlers}
	poll()
	if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did {
		t.Fatal(did, err)
	}
	latest, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
	history, he := transcript.NewStore(f.pool).GetTranscript(f.ctx, "session")
	tail, valid := summary.Tail(history, latest)
	if err != nil || he != nil || !valid || len(tail) != 4 || latest.ThroughSequence <= first.ThroughSequence || !strings.Contains(latest.Summary, "Условие 1") {
		t.Fatal("restart lost exact owner history or raw tail", err, he)
	}
}

// The failed two-call window and its spending stay immutable. A separately
// frozen one-call owner policy exercises recovery of the retained transcript,
// not replay or regrading of the failed quality window.
func TestACPSummaryRecoveryFromRetainedTimeout(t *testing.T) {
	c := acpqualification.Open(t, "summary-recovery-v1", 1)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "acp-summary-recovery-owner"})
	var timeouts int
	if err := c.Pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id='acp-summary-v1' AND a.state='failed' AND a.reason='provider_timeout' AND a.usage IS NULL`).Scan(&timeouts); err != nil || timeouts != 1 {
		t.Fatal("retained original timeout required", err)
	}
	prior, err := summary.NewStore(c.Pool).GetSummarySnapshot(ctx, "session")
	if err != nil || prior.ThroughSequence != 8 {
		t.Fatal("prior valid checkpoint missing", err)
	}
	b := budget.NewController(c.Pool, c.Policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	f := &consumerFixture{pool: c.Pool, cat: catalog.New(c.Pool, c.Manifest), ctx: ctx}
	f.js = jobs.New(c.Pool, f.cat, b, nil)
	if err := f.js.ControlModel(ctx, "acp-summary-v1", "revoked"); err != nil {
		t.Fatal(err)
	}
	if err := f.js.SetModelControl(ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	q := jobs.ModelPolicyRequest{ID: "acp-summary-recovery-v1", Handler: HandlerName, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", AccountingMode: "opaque_subscription", Job: jobs.Request{OriginalRunID: "run", Allowance: c.Allowance(), Deadline: time.Now().Add(20 * time.Minute)}}
	if _, err := (&Policies{Catalog: f.cat, Jobs: f.js}).Create(ctx, PolicyRequest{SessionKey: "session", Policy: q}); err != nil {
		t.Fatal(err)
	}
	w := &jobs.ModelWorker{Jobs: f.js, Provider: c.Provider, Model: "gemini-3.8-flash", Handlers: map[string]jobs.ModelHandler{HandlerName: Consumer{ContextBudget: 4096}}}
	processed := false
	for scan := 0; scan < 20; scan++ {
		if err := f.js.PollModelPolicies(ctx, func(name string) bool { return name == HandlerName }, map[string]jobs.ModelScheduleHandler{HandlerName: Consumer{ContextBudget: 4096}}); err != nil {
			t.Fatal(err)
		}
		ok, err := w.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			processed = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !processed {
		t.Fatal("recovery policy did not schedule")
	}
	latest, err := summary.NewStore(c.Pool).GetSummarySnapshot(ctx, "session")
	history, he := transcript.NewStore(c.Pool).GetTranscript(ctx, "session")
	tail, valid := summary.Tail(history, latest)
	if err != nil || he != nil || !valid || len(tail) != 4 || latest.ThroughSequence != 17 || !strings.Contains(latest.Summary, "Условие 1") || !strings.Contains(latest.Summary, "Условие 16") {
		t.Fatal("recovery lost exact owner constraints or coverage", err, he)
	}
}
