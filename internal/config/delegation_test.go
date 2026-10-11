package config

import "testing"

func TestDelegationTopologyIsOperatorOnlyAndBounded(t *testing.T) {
	env := map[string]string{"BUTLER_POSTGRES_URL": "postgres://localhost:5432/test", "BUTLER_REDIS_URL": "redis://localhost:6379/0"}
	settings := []Setting{{Key: "BUTLER_DELEGATION_ENABLED", Value: "true"}, {Key: "BUTLER_DELEGATION_MAX_PARALLEL", Value: "2"}}
	c, _, err := loadOrchestratorLayered(envMap(env), settings)
	if err != nil || c.DelegationEnabled || c.DelegationMaxParallel != 1 {
		t.Fatal("stored settings changed private delegation topology", c.DelegationEnabled, c.DelegationMaxParallel, err)
	}
	env["BUTLER_DELEGATION_ENABLED"], env["BUTLER_DELEGATION_MAX_PARALLEL"] = "true", "2"
	c, _, err = loadOrchestratorLayered(envMap(env), nil)
	if err != nil || !c.DelegationEnabled || c.DelegationMaxParallel != 2 {
		t.Fatal("operator topology was ignored", err)
	}
	for _, value := range []string{"0", "3", "999999999999999999999999", "-1"} {
		env["BUTLER_DELEGATION_MAX_PARALLEL"] = value
		if _, _, err = loadOrchestratorLayered(envMap(env), nil); err == nil {
			t.Fatal("unbounded executor width accepted", value)
		}
		if _, _, err = loadOrchestrator(envMap(env)); err == nil {
			t.Fatal("base config accepted unbounded executor width", value)
		}
	}
	b, _, err := loadToolBroker(envMap(map[string]string{}))
	if err != nil || b.DelegationEnabled || b.DelegationControlURL != "" {
		t.Fatal("private Broker route enabled by default", err)
	}
	for _, value := range []string{"http://broker/internal/other", "http://user:password@broker/internal/delegation", "http://broker/internal/delegation?q=data", "http://broker/internal/delegation#x", "http://broker/internal/%64elegation", "file:///internal/delegation", "http://broker/internal/delegation/"} {
		if _, _, err = loadToolBroker(envMap(map[string]string{"BUTLER_DELEGATION_CONTROL_URL": value})); err == nil {
			t.Fatal("unsafe authenticated control destination accepted", value)
		}
	}
}
