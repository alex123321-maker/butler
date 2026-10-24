package actionauth

import (
	"context"
	"github.com/butler/butler/internal/communicationcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"strings"
	"testing"
)

func TestOwnerReplyPreservesApprovalModesDestinationsAndDenyOverrides(t *testing.T) {
	for _, scenario := range []string{"mode1", "mode2", "mode3", "denied", "new_destination", "changed_contract"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, c, _, call, _ := fixture(t)
			call.ToolName = "communication.message"
			call.ArgsJson = `{"schema_version":1,"purpose":"result","text":"verified result"}`
			c.current.CommunicationReady = true
			c.current.CommunicationMode = "explicit"
			c.current.CommunicationMethods = []string{call.ToolName}
			c.current.CommunicationTargets = map[string]string{call.ToolName: strings.Repeat("a", 64)}
			c.current.InputClaimHash = InputClaimHash("input", "claim")
			contract := communicationcontrol.Contract(call.ToolName)
			switch scenario {
			case "mode1":
				c.current.AutonomyMode = 2
				call.AutonomyMode = commonv1.AutonomyMode(2)
			case "mode3":
				c.current.AutonomyMode = 4
				call.AutonomyMode = commonv1.AutonomyMode(4)
			case "denied":
				c.current.DenyTools = []string{call.ToolName}
			case "changed_contract":
				contract.RequiresApproval = true
			}
			p, err := s.Prepare(context.Background(), call, contract)
			if scenario == "denied" || scenario == "changed_contract" {
				if err == nil {
					t.Fatal("denied reply prepared")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.RequiresApproval != (scenario == "mode1") {
				t.Fatal("autonomy policy", p.RequiresApproval)
			}
			g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
			if scenario == "mode1" {
				if err == nil {
					t.Fatal("mode1 bypassed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Binding.OwnerCommunication {
				t.Fatal("ordinary result acquired unknown-effect budget exception")
			}
			if scenario == "new_destination" {
				c.current.CommunicationTargets[call.ToolName] = strings.Repeat("b", 64)
				if _, err = s.Verify(context.Background(), call, contract, g.ID); err == nil {
					t.Fatal("rebound target retained grant")
				}
			}
		})
	}
}
