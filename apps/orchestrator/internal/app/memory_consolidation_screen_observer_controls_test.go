package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

func TestConsolidationScreenRejectsHiddenContextBeforeProvider(t *testing.T) {
	for _, test := range []struct{ name, text string }{
		{"oracle", `{"oracle":{"answer":"secret"}}`},
		{"nested_escaped_oracle", `{"outer":"{\"requested_outcome\":\"secret\"}"}`},
		{"case_label", `{"data":"pair-02"}`},
		{"foreign_reference", `{"source_id":"other-original-opaque"}`},
		{"foreign_body", `{"text":"independent foreign case full text"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A nil underlying provider/pool would panic if observation crossed
			// the audit. Both Start and actual tool continuation fail before it.
			p := &consolidationScreenProvider{}
			p.selectSlot(&consolidationScreenSlot{ID: "slot", Kind: "background", State: "begun", Limit: 1}, []string{"other-original-opaque"}, []string{"independent foreign case full text"}, "current")
			ctx := budget.WithModelOperation(context.Background(), "operation")
			if _, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}, InputItems: []transport.InputItem{{Content: test.text}}}); err == nil {
				t.Fatal("hidden Start input accepted")
			}
			if _, err := p.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "run", ToolResultJSON: test.text}); err == nil {
				t.Fatal("hidden continuation input accepted")
			}
			if len(p.calls) != 0 {
				t.Fatal("audit rejection became a paid entry")
			}
		})
	}
	p := &consolidationScreenProvider{}
	p.selectSlot(&consolidationScreenSlot{ID: "slot", Kind: "foreground", State: "begun", Limit: 16}, nil, nil, "current owner fact")
	if _, err := p.StartRun(context.Background(), transport.StartRunRequest{}); err == nil {
		t.Fatal("foreground input without current scoped fact/task accepted")
	}
}

func TestConsolidationScreenAllSlotsSealedBeforeStorage(t *testing.T) {
	proof := filepath.Join(t.TempDir(), "proof")
	_, slots, _ := consolidationScreenSchedule(t, proof)
	if len(slots) != 9 {
		t.Fatal("complete logical population absent")
	}
	for _, slot := range slots {
		if slot.State != "not_executed" || slot.Blocker == "" || slot.ID == slot.CaseID {
			t.Fatal("unexecuted slot lost prospective identity")
		}
	}
}

func TestConsolidationScreenCostsPreserveUnknownAndBackground(t *testing.T) {
	start := time.Now()
	slots := []*consolidationScreenSlot{
		{ID: "bg", CaseID: "case", Kind: "background", Arm: "preparation", StartedAt: start, FinishedAt: start.Add(2 * time.Second)},
		{ID: "raw", CaseID: "case", Kind: "foreground", Arm: "baseline"},
		{ID: "after", CaseID: "case", Kind: "foreground", Arm: "consolidated"},
	}
	calls := []*consolidationScreenCall{
		{SlotID: "bg", Usage: &transport.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 20}},
		{SlotID: "raw", Usage: &transport.Usage{InputTokens: 4, OutputTokens: 1, TotalTokens: 5}, ToolCalls: []string{"memory.open"}},
		{SlotID: "after", Usage: nil, Failure: "provider_unavailable"},
		{SlotID: "after", Usage: &transport.Usage{InputTokens: 2, OutputTokens: 1}},
	}
	costs := consolidationScreenCosts(slots, calls)
	full, after, raw := costs["full_unique_experiment"], costs["after_including_all_preparation"], costs["foreground:baseline"]
	if full.Entries != 4 || full.KnownUsage != 3 || full.UnknownUsage != 1 || full.UnreportedTotal != 1 || full.NonInput != 11 || full.Input != 16 || full.Output != 5 || full.ReportedTotal != 25 || full.ToolCalls != 1 {
		t.Fatal("full usage omitted/zeroed/double-counted", full)
	}
	if after.Entries != 3 || after.UnknownUsage != 1 || after.WallMillis != 2000 || raw.Entries != 1 || after.Input+raw.Input != full.Input {
		t.Fatal("background not charged to after treatment", after, raw)
	}
}

func TestConsolidationScreenDurableEntryBoundsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	slot := &consolidationScreenSlot{ID: "opaque-slot", Kind: "foreground", State: "begun", Limit: 1}
	createConsolidationScreenTables(t, pool, []*consolidationScreenSlot{slot}, []byte(`{}`))
	for _, op := range []string{"first", "second", "third"} {
		if _, err := pool.Exec(ctx, `WITH r AS(INSERT INTO execution_budget_reservations(reservation_id,owner_id,operation_id,fingerprint,request_json) VALUES($1,'owner',$1,'fixture','{}') RETURNING reservation_id) INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance) SELECT $1,'run',reservation_id,'fixture','fixture',100,100 FROM r`, op); err != nil {
			t.Fatal(err)
		}
	}
	p := &consolidationScreenProvider{pool: pool}
	p.selectSlot(slot, nil, nil, "")
	if _, err := p.reserve(budget.WithModelOperation(ctx, "unadmitted"), "run", "tool_result", json.RawMessage(`{}`)); err == nil {
		t.Fatal("context-only operation minted a paid attempt")
	}
	if _, err := p.reserve(budget.WithModelOperation(ctx, "first"), "run", "tool_result", json.RawMessage(`{"data":"safe"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.reserve(budget.WithModelOperation(ctx, "second"), "run", "tool_result", json.RawMessage(`{}`)); err == nil {
		t.Fatal("per-slot bound enlarged")
	}
	var entries, count int
	if err := pool.QueryRow(ctx, `SELECT entries,(SELECT count(*) FROM consolidation_screen_calls) FROM consolidation_screen_window`).Scan(&entries, &count); err != nil || entries != 1 || count != 1 {
		t.Fatal("denied call charged partially", entries, count, err)
	}
	// A fresh permitted logical slot cannot exceed the same one-use window.
	slot.Limit = 16
	if _, err := pool.Exec(ctx, `UPDATE consolidation_screen_window SET entries=99`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.reserve(budget.WithModelOperation(ctx, "third"), "run", "tool_result", json.RawMessage(`{}`)); err == nil {
		t.Fatal("global entry ceiling enlarged")
	}
	if err := pool.QueryRow(ctx, `SELECT entries,(SELECT count(*) FROM consolidation_screen_calls) FROM consolidation_screen_window`).Scan(&entries, &count); err != nil || entries != 99 || count != 1 {
		t.Fatal("global rejection changed ledger", entries, count, err)
	}
}
