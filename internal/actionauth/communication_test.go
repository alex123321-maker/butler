package actionauth

import (
	"context"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/delegation"
	"strings"
	"testing"
)

func TestCommunicationRequiresPinnedCompositionAndExactConsent(t *testing.T) {
	for _, scenario := range []string{"allowed", "disabled", "legacy", "missing_claim", "delegated"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, c, _, call, _ := fixture(t)
			call.ToolName = "communication.message"
			call.ArgsJson = `{"schema_version":1,"purpose":"result","text":"saved"}`
			c.current.CommunicationReady = true
			c.current.CommunicationMethods = []string{"communication.message"}
			c.current.CommunicationTargets = map[string]string{"communication.message": strings.Repeat("a", 64)}
			c.current.CommunicationMode = "explicit"
			c.current.InputClaimHash = InputClaimHash("input", "claim")
			switch scenario {
			case "disabled":
				c.current.CommunicationReady = false
			case "legacy":
				c.current.CommunicationMode = ""
			case "missing_claim":
				c.current.InputClaimHash = ""
			case "delegated":
				c.current.Delegation = &delegation.Contract{}
			}
			contract := communicationcontrol.Contract(call.ToolName)
			p, e := s.Prepare(context.Background(), call, contract)
			if scenario != "allowed" {
				if e == nil {
					t.Fatal("unavailable capability prepared")
				}
				return
			}
			if e != nil || p.Binding.Effect != EffectPrepare || !p.RequiresApproval {
				t.Fatal("publication bypassed authorization", e)
			}
			g := prepareApproved(t, s, c, call, contract)
			if g.InputClaimHash != c.current.InputClaimHash {
				t.Fatal("input claim not pinned")
			}
		})
	}
}
