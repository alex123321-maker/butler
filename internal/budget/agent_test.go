package budget

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentBudgetRequiresExactExplicitSubscriptionBasis(t *testing.T) {
	policy := DefaultPolicy()
	key := "antigravity-acp:gemini-fixture"
	for _, basis := range []string{"", "subscription_no_overage", "local_no_billing", "operator_rate"} {
		policy.Models[key] = ModelPrice{Basis: basis}
		if _, err := policy.AgentQuote("antigravity-acp", "gemini-fixture"); !errors.Is(err, ErrUnknownCost) {
			t.Fatal("agent inference admitted without explicit unknown-token policy", basis, err)
		}
	}
	policy.Models[key] = ModelPrice{Basis: AgentSubscriptionBasis}
	quote, err := policy.ModelQuote("antigravity-acp", "gemini-fixture", 0)
	if err != nil || policy.Validate() != nil || quote.ModelTurns != 1 || quote.ActiveMillis != policy.ModelMillis || quote.ProviderInputBytes != policy.ModelInputBytes || quote.EgressBytes != policy.AgentOutputBytes || quote.Tokens != 0 || quote.CostMicros != 0 {
		t.Fatal("invalid agent reservation", quote, err)
	}
	if _, err := policy.ModelQuote("antigravity-acp", "gemini-fixture", 1000); err == nil {
		t.Fatal("invented agent token allowance accepted")
	}
	allowance := ModelAllowance{Provider: "antigravity-acp", Model: "gemini-fixture", Agent: true}
	if allowance.Validate(policy) != nil {
		t.Fatal("agent preflight does not use the same declared budget")
	}
	allowance.InputTokens = 1
	if allowance.Validate(policy) == nil {
		t.Fatal("agent preflight fabricated a token limit")
	}
	policy.Models["antigravity:gemini-fixture"] = ModelPrice{Basis: AgentSubscriptionBasis}
	if policy.Validate() == nil {
		t.Fatal("opaque metering leaked to the native transport")
	}
}

func TestAgentBudgetDurableUnknownAndRestartIntegration(t *testing.T) {
	pool := controllerDB(t)
	ctx := context.Background()
	for _, migration := range []string{"112_acp_subscription_budget.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	policy := DefaultPolicy()
	policy.Models["antigravity-acp:gemini-fixture"] = ModelPrice{Basis: AgentSubscriptionBasis}
	policy.Task.ModelTurns = 2
	controller := NewController(pool, policy)
	for attempt := 0; attempt < 2; attempt++ {
		admitted, err := controller.AdmitModel(modelCtx("run"), "run", "", "antigravity-acp", "gemini-fixture", 0)
		if err != nil || !admitted.Agent || admitted.ModelTurnLimited || admitted.OutputTokens != 0 {
			t.Fatal("agent was not admitted through durable authority and budget checks", admitted, err)
		}
		controller = NewController(pool, policy)
	}
	_, err := controller.AdmitModel(modelCtx("run"), "run", "", "antigravity-acp", "gemini-fixture", 0)
	var exhausted *ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "model_turns" {
		t.Fatal("restart reset agent spending", err)
	}
	snapshot, err := controller.Ledger.SnapshotOwner(ctx, "owner")
	if err != nil || len(snapshot.Attempts) != 2 {
		t.Fatal("durable attempts missing", snapshot, err)
	}
	for _, attempt := range snapshot.Attempts {
		if attempt.AccountingMode != "opaque_subscription" || attempt.ActualInput != nil || attempt.ActualOutput != nil || attempt.InputAllowance != 0 || attempt.OutputAllowance != 0 {
			t.Fatal("opaque usage presented as a known token measurement", attempt)
		}
	}
	scope, err := controller.Ledger.Get(ctx, ScopeID("task", "owner", "root"))
	if err != nil || scope.Used.ModelTurns != 2 || scope.Used.ActiveMillis != 2*policy.ModelMillis || scope.Used.ProviderInputBytes != 2*policy.ModelInputBytes || scope.Used.EgressBytes != 2*policy.AgentOutputBytes {
		t.Fatal("agent time/byte reservations missing", scope, err)
	}
	usage, err := controller.Ledger.ModelRunUsage(ctx, "owner", "run")
	if err != nil || usage.OpaqueAttempts != 2 || usage.ObservedInput != nil || usage.ObservedOutput != nil {
		t.Fatal("owner receipt misrepresents unknown tokens", usage, err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "112_acp_subscription_budget.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("rollback discarded opaque-accounting history")
	}
}

func TestAgentBudgetPromptLimitSurvivesNativeConversationOptOutIntegration(t *testing.T) {
	pool := ownerDayBudgetDB(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "112_acp_subscription_budget.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_revision) VALUES('conversation','session',0); UPDATE session_ownership SET run_id='conversation'`); err != nil {
		t.Fatal(err)
	}
	p := testPolicy()
	p.DisableOwnerDay, p.DisableConversationModelTurns = true, true
	p.Conversation.ModelTurns = 1
	p.Models["antigravity-acp:gemini-fixture"] = ModelPrice{Basis: AgentSubscriptionBasis}
	controller := NewController(pool, p)
	if _, err = controller.AdmitModel(modelCtx("conversation"), "conversation", "", "antigravity-acp", "gemini-fixture", 0); err != nil {
		t.Fatal(err)
	}
	controller = NewController(pool, p)
	_, err = controller.AdmitModel(modelCtx("conversation"), "conversation", "", "antigravity-acp", "gemini-fixture", 0)
	assertBudgetDimension(t, err, "conversation", "model_turns")
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_model_attempts WHERE provider='antigravity-acp'`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("denied prompt changed spending", attempts, err)
	}
}
