package config

import "testing"

func TestExecutionBudgetPolicyIsValidatedForBothServices(t *testing.T) {
	valid := `{"models":{"antigravity:gemini-3.8-flash-tiered":{"basis":"subscription_no_overage"}}}`
	get := envMap(map[string]string{"BUTLER_EXECUTION_BUDGET_POLICY_JSON": valid, "BUTLER_POSTGRES_URL": "postgres://butler:test@localhost/butler_test", "BUTLER_REDIS_URL": "redis://localhost/0"})
	orch, _, err := loadOrchestrator(get)
	if err != nil {
		t.Fatal(err)
	}
	broker, _, err := loadToolBroker(get)
	if err != nil {
		t.Fatal(err)
	}
	if orch.ExecutionBudgetPolicyJSON != valid || broker.ExecutionBudgetPolicyJSON != valid {
		t.Fatal("services silently disagree on deployment policy")
	}
	for _, invalid := range []string{`{"model_millis":0}`, `{"models":{"x:y":{"basis":"operator_rate"}}}`, `{"unlimited":true}`, `{} {}`} {
		if err = validateBudgetPolicy(invalid); err == nil {
			t.Fatal("invalid policy accepted", invalid)
		}
	}
}
