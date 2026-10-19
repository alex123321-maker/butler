package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestBrowserLocalPreparationJournalAndBudgetIntegration(t *testing.T) {
	s, store, reader, call, contract, _, _ := receiptFixture(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp()`)
	if err != nil {
		t.Fatal(err)
	}
	store.SetBudgets(budget.NewController(store.pool, budget.DefaultPolicy()))
	contract.ContractRevision = "2"
	contract.AllowedDomains = []string{"external.test"}
	version := uint64(1)
	for index, tool := range []string{"browser.open", "browser.prepare_fill", "browser.prepare_type", "browser.click"} {
		contract.ContractRevision = "2"
		call.ToolName, call.ToolCallId, contract.ToolName = tool, fmt.Sprintf("browser-sequence-%d", index), tool
		contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}
		args := map[string]any{"browser_session_id": "context", "expected_url": egresstest.Origin + "/form", "observation_version": version, "selector": "#input", "value": "local-draft", "text": "more"}
		if tool == "browser.open" {
			contract.ContractRevision = "3"
			args = map[string]any{"url": egresstest.Origin + "/form"}
			contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
		} else if tool != "browser.click" {
			contract.EffectContract.VerificationMethod = "local_browser_preparation_v1"
		}
		encoded, _ := json.Marshal(args)
		call.ArgsJson = string(encoded)
		g := logicalGrant(t, s, reader, call, contract, fmt.Sprintf("browser-slot-%d", index))
		if err := store.ClaimGrant(ctx, g.ID); err != nil {
			t.Fatalf("sequence stopped at %s: %v", tool, err)
		}
		resultJSON := fmt.Sprintf(`{"ok":true,"browser_session_id":"context","observation_version":%d,"final_url":%q,"local_preparation":true,"network_state":"frozen"}`, version+1, egresstest.Origin+"/form")
		result := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: tool, Status: "completed", ResultJson: resultJSON}
		effect, err := s.Outcome(ctx, g, result, false)
		if err != nil {
			t.Fatal(err)
		}
		want := StateConfirmed
		if tool == "browser.click" {
			want = StateUnknown
		}
		if effect.State != want || len(effect.EvidenceRefs) != 1 {
			t.Fatalf("%s state %s, expected %s", tool, effect.State, want)
		}
		// The same runtime envelope proves local preparation only; a click does
		// not borrow its proof or clear the root's unknown-effect stop.
		version++
	}
	call.ToolCallId, call.ToolName, contract.ToolName = "after-unknown", "browser.prepare_fill", "browser.prepare_fill"
	call.ArgsJson = fmt.Sprintf(`{"browser_session_id":"context","expected_url":%q,"observation_version":%d,"selector":"#input","value":"forbidden"}`, egresstest.Origin+"/form", version)
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1"}
	g := logicalGrant(t, s, reader, call, contract, "after-unknown-slot")
	var loop *budget.LoopError
	if err := store.ClaimGrant(ctx, g.ID); !errors.As(err, &loop) || loop.Reason != "external_effect_unknown" {
		t.Fatal("unknown external outcome did not block later preparation", err)
	}
	// Restart preserves the evidence and does not make the old dispatch grant
	// reusable. There is no caller-supplied reconciliation shortcut.
	restarted := NewPostgresStore(store.pool)
	effects, err := restarted.GetRunEvidence(ctx, call.RunId)
	if err != nil || len(effects) != 5 {
		t.Fatal("journal lost across restart", err, len(effects))
	}
	var unknown int
	for _, effect := range effects {
		if effect.EffectState == StateUnknown {
			unknown++
		}
	}
	if unknown != 1 {
		t.Fatal("final submission uncertainty changed", unknown)
	}
	var reservations int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 4 {
		t.Fatal("denied follow-up charged or dispatched", err, reservations)
	}
}
