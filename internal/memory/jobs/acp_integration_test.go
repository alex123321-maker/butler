package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
)

// This fixture deliberately has no InferenceLimits method. Opaque model usage
// must remain unknown and must not panic if an unqualified usage event appears.
type opaqueMemoryFixture struct {
	transport.ModelProvider
	source catalog.Source
	before func()
	usage  bool
}

func TestOpaqueMemoryAccountingModeMismatchWaitsWithoutSpendIntegration(t *testing.T) {
	w, ctx, src := modelFixture(t)
	w.Model = "gemini-fixture"
	w.Jobs.budgets.Policy.Models["antigravity-acp:gemini-fixture"] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
	w.Provider = &opaqueMemoryFixture{source: src}
	j := schedule(t, w, ctx, policyRequest(src)) // Existing token policy is immutable.
	if err := w.execute(ctx, j, QuoteProbe{}); !errors.Is(err, ErrWaiting) {
		t.Fatal("old token policy was reinterpreted", err)
	}
	var status, reason string
	var attempts, spent int
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT status,error_code,attempts FROM memory_aux_jobs WHERE job_id=$1`, j.ID).Scan(&status, &reason, &attempts); err != nil || status != "pending" || reason != "accounting_mode_mismatch" || attempts != 0 {
		t.Fatal("mode mismatch consumed retry", status, reason, attempts, err)
	}
	if err := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&spent); err != nil || spent != 0 {
		t.Fatal("mode mismatch charged", spent, err)
	}
}

func TestModelPolicyAccountingModes(t *testing.T) {
	base := budget.Units{ModelTurns: 1, ActiveMillis: 300000, ProviderInputBytes: 8388608, EgressBytes: 65536}
	for _, mode := range []string{"", "tokens", "opaque_subscription", "unknown"} {
		q := ModelPolicyRequest{AccountingMode: mode, Job: Request{Allowance: base}}
		if validModelAllowance(q) != (mode == "opaque_subscription") {
			t.Fatal("zero tokens selected implicit mode", mode)
		}
	}
	for _, field := range []string{"tokens", "cost", "calls", "children", "output", "input", "turns", "time"} {
		q := ModelPolicyRequest{AccountingMode: "opaque_subscription", Job: Request{Allowance: base}}
		switch field {
		case "tokens":
			q.Job.Allowance.Tokens = 1
		case "cost":
			q.Job.Allowance.CostMicros = 1
		case "calls":
			q.Job.Allowance.ExternalCalls = 1
		case "children":
			q.Job.Allowance.Children = 1
		case "output":
			q.Job.Allowance.EgressBytes = 0
		case "input":
			q.Job.Allowance.ProviderInputBytes = 0
		case "turns":
			q.Job.Allowance.ModelTurns = 0
		case "time":
			q.Job.Allowance.ActiveMillis = 0
		}
		if validModelAllowance(q) {
			t.Fatal("invalid opaque allowance", field)
		}
	}
}

func (*opaqueMemoryFixture) Name() string { return "antigravity-acp" }
func (*opaqueMemoryFixture) AgentExecutionLimits() transport.AgentExecutionLimits {
	return transport.AgentExecutionLimits{InputBytes: 1024, OutputBytes: 1024, WallTime: time.Second}
}
func (*opaqueMemoryFixture) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *opaqueMemoryFixture) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	if p.before != nil {
		p.before()
	}
	var usage *transport.Usage
	if p.usage {
		usage = &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(jsonBytes(map[string]string{"source_id": p.source.ID, "quote": p.source.Body})), Usage: usage})
	close(ch)
	return ch, nil
}

func TestOpaqueMemoryPreservesPublicationUnknownAndSpentBudgetIntegration(t *testing.T) {
	for _, mode := range []string{"published", "revoked", "unexpected_usage"} {
		t.Run(mode, func(t *testing.T) {
			w, ctx, src := modelFixture(t)
			w.Model = "gemini-fixture"
			w.Jobs.budgets.Policy.Models["antigravity-acp:gemini-fixture"] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
			p := &opaqueMemoryFixture{source: src}
			w.Provider = p
			q := policyRequest(src)
			q.AccountingMode = "opaque_subscription"
			q.Job.Allowance.Tokens = 0
			q.Job.Allowance.EgressBytes = 65536
			j := schedule(t, w, ctx, q)
			if mode == "revoked" {
				p.before = func() {
					if err := w.Jobs.ControlModel(ctx, q.ID, "revoked"); err != nil {
						t.Fatal(err)
					}
				}
			}
			p.usage = mode == "unexpected_usage"
			err := w.execute(ctx, j, QuoteProbe{})
			if (err == nil) != (mode == "published") {
				t.Fatal("unexpected opaque result", mode, err)
			}
			var used budget.Units
			var raw []byte
			if err := w.Jobs.pool.QueryRow(ctx, `SELECT quote FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if json.Unmarshal(raw, &used) != nil || used.ModelTurns != 1 || used.Tokens != 0 || used.ActiveMillis != 1000 || used.EgressBytes != 65536 {
				t.Fatal("opaque reservation missing", string(raw))
			}
			var unknown bool
			var state string
			if err := w.Jobs.pool.QueryRow(ctx, `SELECT usage IS NULL,state FROM memory_model_attempts WHERE job_id=$1`, j.ID).Scan(&unknown, &state); err != nil || !unknown || mode == "published" && state != "published" || mode != "published" && state == "published" {
				t.Fatal("unknown usage or publication fence lost", mode, state, err)
			}
			// New worker/controller must consume the same durable final unit.
			w.Jobs.budgets = budget.NewController(w.Jobs.pool, w.Jobs.budgets.Policy)
			var count int
			if err := w.Jobs.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count); err != nil || count != 1 {
				t.Fatal("charge lost", count, err)
			}
		})
	}
}
