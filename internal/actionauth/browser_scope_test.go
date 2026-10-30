package actionauth

import (
	"context"
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type browserScopeFixture struct {
	resource string
	epoch    int64
}

func (f *browserScopeFixture) BrowserReadScope(context.Context, ActionBinding) (string, int64, error) {
	return f.resource, f.epoch, nil
}

func TestOwnerGoalBrowserContextReadScopeAndEpoch(t *testing.T) {
	for _, scenario := range []string{"allowed", "missing_reader", "foreign_url", "credential_contract", "epoch_changed", "deny_resource"} {
		t.Run(scenario, func(t *testing.T) {
			s, store, c, _, call, _ := fixture(t)
			m := testOwnerMandate(s.now())
			m.OwnerID = c.current.OwnerID
			m.Tools = append(m.Tools, "browser.snapshot")
			c.current.Authority = &Assignment{TaskID: c.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			c.current.MandateID = m.ID
			if scenario == "deny_resource" {
				m.DenyResources = m.ResourceScopes
			}
			store.mandates[m.ID] = m
			call.ToolName = "browser.snapshot"
			call.ArgsJson = `{"browser_session_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
			contract := &toolbrokerv1.ToolContract{ToolName: call.ToolName, ContractRevision: "3", InputSchemaJson: `{"type":"object"}`, Status: "enabled"}
			reader := &browserScopeFixture{resource: "https://example.invalid/test-service/page", epoch: 2}
			if scenario == "foreign_url" {
				reader.resource = "https://foreign.invalid/"
			}
			if scenario == "credential_contract" {
				contract.SupportsCredentialRefs = true
			}
			if scenario != "missing_reader" {
				s.SetBrowserScopes(reader)
			}
			p, err := s.Prepare(context.Background(), call, contract)
			if scenario != "allowed" && scenario != "epoch_changed" {
				if err == nil {
					t.Fatal("unsafe context permission")
				}
				return
			}
			if err != nil || p.RequiresApproval || p.Binding.BrowserScopeURL != reader.resource || p.Binding.BrowserScopeEpoch != 2 {
				t.Fatal("missing exact owned context scope", err)
			}
			g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "epoch_changed" {
				reader.epoch++
			}
			_, err = s.Verify(context.Background(), call, contract, g.ID)
			if (err == nil) != (scenario == "allowed") {
				t.Fatal("changed context epoch kept grant", err)
			}
		})
	}
}
