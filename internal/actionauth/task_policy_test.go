package actionauth

import (
	"context"
	"testing"
)

func TestPrepareOnlyDoesNotAuthorizeExternalEffects(t *testing.T) {
	for _, effect := range []Effect{EffectRead, EffectPrepare, EffectCommit, EffectUnknown} {
		err := CheckExecutionPolicy(ContextSnapshot{ExecutionPolicy: "prepare_only"}, effect)
		if (effect == EffectRead) != (err == nil) {
			t.Fatalf("effect %s: %v", effect, err)
		}
	}
	if CheckExecutionPolicy(ContextSnapshot{ExecutionPolicy: "unknown_policy"}, EffectRead) == nil {
		t.Fatal("unknown policy allowed")
	}
}

func TestPrepareOnlyRevokesPreviouslyApprovedGrant(t *testing.T) {
	s, _, c, _, call, contract := fixture(t)
	grant := prepareApproved(t, s, c, call, contract)
	c.current.ExecutionPolicy = "prepare_only"
	if _, err := s.Verify(context.Background(), call, contract, grant.ID); err == nil {
		t.Fatal("approved mutation survived restrictive policy")
	}
	if _, err := s.Authorize(context.Background(), call, contract, grant.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("approval overrode prepare-only policy")
	}
	if _, err := s.Prepare(context.Background(), call, contract); err == nil {
		t.Fatal("new write intent allowed under prepare-only policy")
	}
}
