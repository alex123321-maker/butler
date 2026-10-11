package actionauth

import (
	"context"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestGuardedRequestGrantPinsFreshClaimWithoutChangingApproval(t *testing.T) {
	s, _, c, _, call, _ := fixture(t)
	call.ToolName, call.ArgsJson = "single_tab.bind.guarded", "{}"
	contract := &toolbrokerv1.ToolContract{ToolName: call.ToolName, ContractRevision: "1", Status: "active", InputSchemaJson: `{"type":"object"}`, RuntimeTarget: "broker://guarded-tab-request-v1"}
	c.current.GuardedTabRequestReady = true
	c.current.InputClaimHash = InputClaimHash("input", "first-worker")
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: prepared.Binding, ExpiresAt: s.now().Add(time.Minute), ResolvedAt: s.now()}
	c.current.InputClaimHash = InputClaimHash("input", "fresh-worker")
	grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "approval", "")
	if err != nil || grant.InputClaimHash != c.current.InputClaimHash || grant.Binding.ActionHash != prepared.Binding.ActionHash {
		t.Fatal("fresh claim changed immutable approved intent", err)
	}
	c.current.InputClaimHash = ""
	if _, err = s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("missing current claim authorized guarded request")
	}
}
