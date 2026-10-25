package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestConverterLocalNativeReservationsCoverModelAndToolQuotesIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	ledger := budget.NewLedger(pool)
	cfg := converterLocalNativeConfig()
	model, err := cfg.Policy.ModelQuote(cfg.Provider, cfg.Model, cfg.InputTokens)
	if err != nil || cfg.Policy.Validate() != nil {
		t.Fatal("native policy quote invalid", err)
	}
	tool := budget.Units{ExternalCalls: 1, ActiveMillis: cfg.Policy.ToolMillis, EgressBytes: cfg.Policy.ToolEgressBytes}
	deadline := time.Now().Add(20 * time.Minute).UTC().Truncate(time.Microsecond)
	spec := budget.ScopeSpec{Kind: "task", OwnerID: "quote-owner", SubjectID: "old-budget", Limits: cfg.Policy.Task, Deadline: deadline}
	spec.Limits.ActiveMillis = 2400000
	old, err := ledger.Ensure(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	reserve := func(id, operation string, charge budget.Units) error {
		_, _, err := ledger.Reserve(ctx, budget.Request{OwnerID: spec.OwnerID, OperationID: operation, ScopeIDs: []string{id}, Charge: charge, CostKnown: true})
		return err
	}
	// Reproduce B's11 model +9 tool reservations without another HTTP call.
	for i := 0; i < 20; i++ {
		charge := model
		if i < 9 {
			charge = tool
		}
		if err = reserve(old.ID, fmt.Sprintf("old-%d", i), charge); err != nil {
			t.Fatal(err)
		}
	}
	var exhausted *budget.ExhaustedError
	if err = reserve(old.ID, "old-next-model", model); !errors.As(err, &exhausted) || exhausted.Dimension != "active_millis" {
		t.Fatal("old task did not retain its exact reservation stop", err)
	}
	spec.Limits = cfg.Policy.Task
	if _, err = ledger.Ensure(ctx, spec); !errors.Is(err, budget.ErrConflict) {
		t.Fatal("new defaults expanded an old task", err)
	}
	retained, err := ledger.Get(ctx, old.ID)
	if err != nil || retained.Used.ActiveMillis != 2400000 {
		t.Fatal("old budget was refunded or rewritten", retained, err)
	}
	spec.SubjectID = "new-budget"
	current, err := ledger.Ensure(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	// Complete deterministic workflow:14 model admissions and10 exact actions.
	for i := 0; i < 24; i++ {
		charge := model
		if i < 10 {
			charge = tool
		}
		if err = reserve(current.ID, fmt.Sprintf("new-%d", i), charge); err != nil {
			t.Fatal("complete native quote inventory cannot fit", err)
		}
	}
	current, err = ledger.Get(ctx, current.ID)
	if err != nil || current.Used.ModelTurns != 14 || current.Used.ExternalCalls != 10 || current.Used.ActiveMillis != 14*model.ActiveMillis+10*tool.ActiveMillis {
		t.Fatal("model/tool quotes were discounted", current, err)
	}
	if cfg.Policy.Task.ActiveMillis < cfg.Policy.Task.ModelTurns*model.ActiveMillis+20*tool.ActiveMillis || cfg.Policy.Mandate.ActiveMillis < cfg.Policy.Mandate.ModelTurns*model.ActiveMillis+20*tool.ActiveMillis || cfg.ChildBudget.ActiveMillis < cfg.ChildBudget.ModelTurns*model.ActiveMillis+cfg.ChildBudget.ExternalCalls*tool.ActiveMillis {
		t.Fatal("prospective finite caps omit tool reservations")
	}
}
