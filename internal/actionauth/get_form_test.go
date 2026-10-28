package actionauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestGetFormReadContractCannotBorrowWriteOrCredentialPolicy(t *testing.T) {
	b := ActionBinding{ToolName: "browser.submit_get_form", ContractRevision: "2", Effect: EffectRead}
	c := &toolbrokerv1.ToolContract{EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "guarded_browser_get_v1"}}
	if classifyEffect(b.ToolName, nil) != EffectRead {
		t.Fatal("GET form misclassified")
	}
	if _, err := ResolveEffectPolicy(b, c); err != nil {
		t.Fatal(err)
	}
	c.EffectContract.VerificationMethod = "none"
	if _, err := ResolveEffectPolicy(b, c); err == nil {
		t.Fatal("opaque GET form contract accepted")
	}
	c.EffectContract.VerificationMethod = BrowserGETVerification
	c.SupportsCredentialRefs = true
	if _, err := ResolveEffectPolicy(b, c); err == nil {
		t.Fatal("credential GET form accepted")
	}
	c.SupportsCredentialRefs = false
	b.ContractRevision = "1"
	if _, err := ResolveEffectPolicy(b, c); err == nil {
		t.Fatal("unreviewed GET form revision accepted")
	}
}

func TestGetFormReadReleaseRequiresCurrentPublicConsent(t *testing.T) {
	s, _, _, _, call, contract := fixture(t)
	call.ToolName, contract.ToolName = "browser.submit_get_form", "browser.submit_get_form"
	contract.ContractRevision = "2"
	contract.SupportsCredentialRefs = false
	contract.AllowedDomains = nil
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "guarded_browser_get_v1"}
	call.ArgsJson = `{"expected_url":"https://example.com/search","fields":[{"name":"q","value":"query"}]}`
	if required, err := s.networkReleaseRequired(call, contract); err != nil || !required {
		t.Fatal("GET query released without standing consent", err)
	}
	s.SetPublicWebReads(true)
	if required, err := s.networkReleaseRequired(call, contract); err != nil || required {
		t.Fatal("approved public GET query blocked", err)
	}
	contract.ContractRevision = "1"
	if required, err := s.networkReleaseRequired(call, contract); err != nil || !required {
		t.Fatal("unreviewed revision borrowed read consent", err)
	}
	contract.ContractRevision = "2"
	call.ArgsJson = `{"expected_url":"http://127.0.0.1/search","fields":[]}`
	if _, err := s.networkReleaseRequired(call, contract); err == nil {
		t.Fatal("private form resource borrowed public consent")
	}
}

func TestBrowserGetFormReadKeepsUnknownWriteBlockedIntegration(t *testing.T) {
	testBrowserReadKeepsUnknownWriteBlocked(t, "browser.submit_get_form")
}

func TestFrozenObservationKeepsUnknownWriteBlockedIntegration(t *testing.T) {
	testBrowserReadKeepsUnknownWriteBlocked(t, "browser.freeze_snapshot")
}

func testBrowserReadKeepsUnknownWriteBlocked(t *testing.T, readTool string) {
	s, store, reader, call, contract, _, _ := receiptFixture(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	store.SetBudgets(budget.NewController(store.pool, budget.DefaultPolicy()))
	call.ToolName, call.ToolCallId, contract.ToolName = "browser.click", "old-click", "browser.click"
	contract.ContractRevision = "3"
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}
	call.ArgsJson = `{"browser_session_id":"old-context","expected_url":"https://example.com/form","observation_version":1,"selector":"button"}`
	old := logicalGrant(t, s, reader, call, contract, "unknown-click-slot")
	if err = store.ClaimGrant(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	result := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: `{"ok":true}`}
	effect, err := s.Outcome(ctx, old, result, false)
	if err != nil || effect.State != StateUnknown {
		t.Fatal("opaque click lost uncertainty", err)
	}
	oldEffectID := effect.ActionID
	call.ToolName, call.ToolCallId, contract.ToolName = "browser.submit_get_form", "read-form", "browser.submit_get_form"
	contract.ContractRevision = "2"
	contract.SupportsCredentialRefs = false
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "guarded_browser_get_v1"}
	call.ArgsJson = `{"browser_session_id":"new-context","expected_url":"https://example.com/catalog","observation_version":1,"selector":"form","fields":[{"name":"q","value":"public query"}]}`
	if readTool == "browser.freeze_snapshot" {
		call.ToolName, contract.ToolName, contract.ContractRevision = readTool, readTool, "1"
		contract.EffectContract.VerificationMethod = "read_result"
		call.ArgsJson = `{"browser_session_id":"old-context"}`
	}
	next := logicalGrant(t, s, reader, call, contract, "get-form-slot")
	if err = store.ClaimGrant(ctx, next.ID); err != nil {
		t.Fatal("independent GET observation blocked", err)
	}
	result.ToolCallId, result.ToolName = call.ToolCallId, call.ToolName
	effect, err = s.Outcome(ctx, next, result, false)
	if err != nil || effect.State != StateConfirmed {
		t.Fatal("GET read not recorded", err)
	}
	retained, err := store.GetEffect(ctx, oldEffectID)
	if err != nil || retained.State != StateUnknown {
		t.Fatal("GET observation reconciled old write", err)
	}
	if err = store.ClaimGrant(ctx, old.ID); err == nil {
		t.Fatal("GET observation revived consumed grant")
	}
	call.ToolName, call.ToolCallId, contract.ToolName = "browser.click", "another-click", "browser.click"
	contract.ContractRevision = "3"
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}
	call.ArgsJson = `{"browser_session_id":"new-context","expected_url":"https://example.com/catalog","observation_version":2,"selector":"different-button"}`
	denied := logicalGrant(t, s, reader, call, contract, "blocked-new-click")
	var stopped *budget.LoopError
	if err = store.ClaimGrant(ctx, denied.ID); !errors.As(err, &stopped) || stopped.Reason != "external_effect_unknown" {
		t.Fatal("GET form bypassed mutation isolation", err)
	}
	var reservations int
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 2 {
		t.Fatal("read refunded usage or denied mutation was charged", err, reservations)
	}
}
