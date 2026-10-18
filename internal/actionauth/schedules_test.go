package actionauth

import (
	"context"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/schedulecontrol"
	"strings"
	"testing"
)

func TestSchedulesRequireCurrentCompositionClaimAndExactConsent(t *testing.T) {
	for _, scenario := range []string{"read", "mutate", "disabled", "missing_claim", "delegated", "changed_claim"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, c, _, call, _ := fixture(t)
			call.ToolName = "schedule.list"
			call.ArgsJson = "{}"
			c.current.SchedulesReady = true
			c.current.InputClaimHash = InputClaimHash("input", "claim")
			if scenario == "mutate" {
				call.ToolName = "schedule.cancel"
				call.ArgsJson = `{"schedule_id":"sch_00000000000000000000000000000000","expected_revision":1}`
			}
			if scenario == "disabled" {
				c.current.SchedulesReady = false
			}
			if scenario == "missing_claim" {
				c.current.InputClaimHash = ""
			}
			if scenario == "delegated" {
				c.current.Delegation = &delegation.Contract{}
			}
			contract := schedulecontrol.Contract(call.ToolName)
			prepared, err := s.Prepare(context.Background(), call, contract)
			denied := scenario == "disabled" || scenario == "missing_claim" || scenario == "delegated"
			if denied {
				if err == nil {
					t.Fatal("unavailable schedule capability prepared")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "mutate" && !prepared.RequiresApproval {
				t.Fatal("schedule mutation bypassed consent")
			}
			if scenario == "read" && prepared.Binding.Effect != EffectRead {
				t.Fatal("schedule inspection is not read-only")
			}
			grant := prepareApproved(t, s, c, call, contract)
			if grant.InputClaimHash != c.current.InputClaimHash {
				t.Fatal("owning claim not pinned")
			}
			if scenario == "changed_claim" {
				c.current.InputClaimHash = strings.Repeat("a", 64)
				if _, err = s.Verify(context.Background(), call, contract, grant.ID); err == nil {
					t.Fatal("old ingress claim reused")
				}
			}
		})
	}
}
