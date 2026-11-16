package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/acpqualification"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

func TestACPLiveCancellationAndPolicyRevocation(t *testing.T) {
	c := acpqualification.Open(t, "cancel-v1", 2)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "acp-cancel-proof"})
	cat := catalog.New(c.Pool, c.Manifest)
	b := budget.NewController(c.Pool, c.Policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	s := New(c.Pool, cat, b, nil)
	if err := s.SetModelControl(ctx, "enabled"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"cancel", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			src, err := cat.Put(ctx, catalog.PutSource{ExternalKey: "acp-withdraw-" + mode, Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Synthetic withdrawal", Body: "Синтетический источник: эффект неизвестен; повторная отправка запрещена."})
			if err != nil {
				t.Fatal(err)
			}
			q := policyRequest(src)
			q.ID = "acp-withdraw-" + mode
			q.AccountingMode = "opaque_subscription"
			q.MaxJobs = 1
			q.Job.ID = ""
			q.Job.Allowance = c.Allowance()
			w := &ModelWorker{Jobs: s, Provider: c.Provider, Model: "gemini-3.8-flash", Handlers: map[string]ModelHandler{"source-quote-v1": QuoteProbe{}}}
			if _, err := s.CreateModelPolicy(ctx, q); err != nil {
				t.Fatal(err)
			}
			var j Job
			claimed := false
			for scan := 0; scan < 20; scan++ {
				if err := s.PollModelPolicies(ctx, func(name string) bool { return name == "source-quote-v1" }); err != nil {
					t.Fatal(err)
				}
				j, claimed, err = s.ClaimPurpose(ctx, "model", "source-quote-v1")
				if err != nil {
					t.Fatal(err)
				}
				if claimed {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			if !claimed {
				t.Fatal("ordinary scanner did not admit withdrawal case")
			}
			callCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() { defer close(finished); done <- w.execute(callCtx, j, QuoteProbe{}) }()
			defer func() { stop(); <-finished }()
			until := time.NewTimer(30 * time.Second)
			defer until.Stop()
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				var dispatched bool
				if err := c.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_model_attempts WHERE job_id=$1 AND state='dispatched')`, j.ID).Scan(&dispatched); err != nil {
					t.Fatal(err)
				}
				if dispatched {
					break
				}
				select {
				case err := <-done:
					t.Fatal("provider terminated before withdrawal", err)
				case <-until.C:
					t.Fatal("dispatch not observed")
				case <-tick.C:
				}
			}
			if mode == "cancel" {
				err = s.CancelModelJob(ctx, j.ID)
			} else {
				err = s.ControlModel(ctx, q.ID, "revoked")
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("withdrawn model published")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("real ACP did not stop after withdrawal")
			}
			var unknown bool
			var state, result string
			if err := c.Pool.QueryRow(ctx, `SELECT usage IS NULL,state,COALESCE(result_source_id,'') FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&unknown, &state, &result); err != nil || !unknown || state == "published" || result != "" {
				t.Fatal("withdrawal released a result", state, err)
			}
			var spent int
			if err := c.Pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations r JOIN memory_model_attempts a USING(reservation_id) WHERE a.job_id=$1`, j.ID).Scan(&spent); err != nil || spent != 1 {
				t.Fatal("withdrawal refunded spend", spent, err)
			}
		})
	}
}
