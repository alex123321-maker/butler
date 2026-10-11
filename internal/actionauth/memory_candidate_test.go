package actionauth

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/candidatecontrol"
)

func TestMemoryCandidateGrantPinsCurrentClaimAndReadiness(t *testing.T) {
	s, _, c, _, call, _ := fixture(t)
	call.ToolName = candidatecontrol.Tool
	call.ArgsJson = `{"source_id":"source","source_revision":1,"fact_key":"language","value_json":"\"ru\"","summary":"candidate"}`
	contract := candidatecontrol.Contract()
	c.current.InputClaimHash = InputClaimHash("input", "original")
	if _, err := s.Prepare(context.Background(), call, contract); err == nil {
		t.Fatal("unavailable owning composition authorized")
	}
	c.current.MemoryCandidateReady = true
	prepared, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: prepared.Binding, ExpiresAt: s.now().Add(time.Minute), ResolvedAt: s.now()}
	c.current.InputClaimHash = InputClaimHash("input", "fresh")
	grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "approval", "")
	if err != nil || grant.InputClaimHash != c.current.InputClaimHash || grant.Binding.ActionHash != prepared.Binding.ActionHash {
		t.Fatal("claim pin changed immutable approved intent", err)
	}
	c.current.InputClaimHash = ""
	if _, err = s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("missing owning claim authorized candidate")
	}
}
