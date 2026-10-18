package actionauth

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/readcontrol"
)

func TestMemoryReadRequiresCurrentCompositionAndClaim(t *testing.T) {
	for _, tool := range []string{readcontrol.Search, readcontrol.Open, readcontrol.Neighbors} {
		s, _, c, _, call, _ := fixture(t)
		call.ToolName = tool
		call.ArgsJson = `{"query":"source"}`
		c.current.InputClaimHash = InputClaimHash("input", "claim")
		if _, err := s.Prepare(context.Background(), call, readcontrol.Contract(tool)); err == nil {
			t.Fatal("absent read composition admitted")
		}
		c.current.MemoryReadReady = true
		p, err := s.Prepare(context.Background(), call, readcontrol.Contract(tool))
		if err != nil || p.Binding.Effect != EffectRead {
			t.Fatal("memory read changed effect/failed", err)
		}
		c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: p.Binding, ExpiresAt: s.now().Add(time.Minute), ResolvedAt: s.now()}
		g, err := s.Authorize(context.Background(), call, readcontrol.Contract(tool), p.Binding.ActionID, "approval", "")
		if err != nil || g.InputClaimHash != c.current.InputClaimHash {
			t.Fatal("read grant lost input claim", err)
		}
		c.current.InputClaimHash = InputClaimHash("input", "changed")
		if _, err = s.Verify(context.Background(), call, readcontrol.Contract(tool), g.ID); err == nil {
			t.Fatal("changed input retained read grant")
		}
		c.current.InputClaimHash = ""
		if _, err := s.Prepare(context.Background(), call, readcontrol.Contract(tool)); err == nil {
			t.Fatal("claimless read prepared")
		}
	}
}
