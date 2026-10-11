package actionauth

import (
	"context"
	"testing"
	"time"
)

func TestWorkspaceGrantPinsOriginalInputAndUnavailableReadiness(t *testing.T) {
	s, _, c, _, call, contract := fixture(t)
	call.ToolName = "workspace.create"
	call.ArgsJson = `{}`
	contract.ToolName = call.ToolName
	c.current.InputClaimHash = InputClaimHash("input", "original")
	if _, err := s.Prepare(context.Background(), call, contract); err == nil {
		t.Fatal("unavailable workspace runtime authorized")
	}
	c.current.WorkspaceReady = true
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	if p.Binding.Effect != EffectPrepare {
		t.Fatal("workspace artifact preparation was not bounded preparation")
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: p.Binding, ExpiresAt: s.now().Add(time.Minute), ResolvedAt: s.now()}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "approval", "")
	if err != nil || g.InputClaimHash != c.current.InputClaimHash {
		t.Fatal("original workspace claim not pinned", err)
	}
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err != nil {
		t.Fatal("current exact grant denied", err)
	}
	c.current.InputClaimHash = InputClaimHash("input", "changed")
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("new input claim inherited old workspace grant")
	}
	c.current.InputClaimHash = ""
	if _, err = s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("workspace grant without current input")
	}
}
