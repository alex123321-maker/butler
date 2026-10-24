package actionauth

import (
	"context"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/schedulecontrol"
	"strings"
	"testing"
	"time"
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

func TestSchedulesAreStandardForAssignedOwnerMandates(t *testing.T) {
	for _, contract := range schedulecontrol.Contracts() {
		t.Run(contract.ToolName, func(t *testing.T) {
			s, store, c, inspection, call, _ := fixture(t)
			m := testOwnerMandate(s.now())
			m.OwnerID = c.current.OwnerID
			m.Tools = []string{"memory.search"}
			m.ResourceScopes = []ResourceScope{{Origin: "tool:memory.search"}}
			m.Effects = []Effect{EffectRead, EffectPrepare}
			store.mandates[m.ID] = m
			c.current.Authority = &Assignment{TaskID: c.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			c.current.MandateID = m.ID
			c.current.SchedulesReady = true
			c.current.InputClaimHash = InputClaimHash("input", "claim")
			call.ToolName, call.ArgsJson = contract.ToolName, `{}`
			prepared, err := s.Prepare(context.Background(), call, contract)
			if err != nil || prepared.RequiresApproval {
				t.Fatal("ordinary existing mandate did not authorize schedule control", err)
			}
			grant, err := s.Authorize(context.Background(), call, contract, prepared.Binding.ActionID, "", "")
			if err != nil || grant.ApprovalID != "" || grant.MandateID != m.ID {
				t.Fatal("schedule control required separate approval/mandate", err)
			}
			if _, err = s.Verify(context.Background(), call, contract, grant.ID); err != nil || inspection.calls != 0 {
				t.Fatal("default control lost current authority or requested credentials", err)
			}
		})
	}
}

func TestSchedulesStandardCapabilityPreservesAuthorityBounds(t *testing.T) {
	now := time.Now().UTC()
	for _, scenario := range []string{"allowed", "tool_denied", "resource_denied", "read_only", "stale_revision", "paused", "expired", "wrong_purpose", "wrong_resource", "wrong_contract", "credential", "unknown_tool"} {
		t.Run(scenario, func(t *testing.T) {
			m := testOwnerMandate(now)
			m.Effects = []Effect{EffectRead, EffectPrepare}
			hash, _ := ContractHash(schedulecontrol.Contract("schedule.cancel"))
			b := ActionBinding{OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose, ToolName: "schedule.cancel", Resource: "tool:schedule.cancel", ContractHash: hash, Effect: EffectPrepare}
			switch scenario {
			case "tool_denied":
				m.DenyTools = []string{b.ToolName}
			case "resource_denied":
				m.DenyResources = []ResourceScope{{Origin: b.Resource}}
			case "read_only":
				m.Effects = []Effect{EffectRead}
			case "stale_revision":
				b.MandateRevision++
			case "paused":
				m.Status = "paused"
			case "expired":
				m.ExpiresAt = now.Add(-time.Second)
			case "wrong_purpose":
				b.Purpose = "another-purpose"
			case "wrong_resource":
				b.Resource = "https://other.invalid"
			case "wrong_contract":
				b.ContractHash = strings.Repeat("f", 64)
			case "credential":
				b.CredentialRefs = []CredentialRef{{Alias: "secret", Field: "password"}}
			case "unknown_tool":
				b.ToolName = "schedule.execute_arbitrary"
			}
			err := CheckAssignedAction(m, b, now)
			if (scenario == "allowed") != (err == nil) {
				t.Fatal("standard control changed scope or ignored a denial", err)
			}
		})
	}
	m := testOwnerMandate(now)
	before := len(m.Tools)
	if len(EffectiveMandateTools(m)) != before+12 || len(m.Tools) != before {
		t.Fatal("effective kernel tools mutated stored mandate")
	}
	m.ScopeKind = ""
	if len(EffectiveMandateTools(m)) != before+2 {
		t.Fatal("legacy task acquired non-catalog capabilities")
	}
}
