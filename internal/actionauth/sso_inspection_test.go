package actionauth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

func TestSSOInspectionRequiresCurrentAllOfAuthority(t *testing.T) {
	for _, mode := range []string{"allowed", "RP_only", "IdP_only", "deny_IdP", "deny_verify", "wrong_RP", "foreign_run", "no_task", "no_mandate", "credentials", "changed_contract", "private_IdP", "revoked_after_prepare", "changed_epoch"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, _, call, _ := fixture(t)
			reader.current.AutonomyMode = 3
			call.AutonomyMode = 3 // enum MODE_2; exact current owner-goal read authority
			base := ssoTestScope()
			a := browsersso.Inspection{Browser: base.Browser, RPPage: base.RPPage, RPURL: base.RPURL, Trigger: "button >> nth=0", IdPOrigin: "https://idp.example", VerifyURL: base.VerifyURL}
			if mode == "private_IdP" {
				a.IdPOrigin = "https://127.0.0.1"
			}
			page := &pageScopeFixture{bound: browserpage.Binding{Ref: a.RPPage, URL: a.RPURL, Epoch: 2, RuntimeVersion: 5}}
			s.SetBrowserScopes(page)
			m := testOwnerMandate(s.now())
			m.OwnerID = reader.current.OwnerID
			m.Tools = []string{browsersso.InspectTool}
			m.Effects = []Effect{EffectRead}
			m.CredentialRefs = nil
			m.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: a.IdPOrigin, PathPrefix: "/"}}
			if mode == "RP_only" {
				m.ResourceScopes = m.ResourceScopes[:1]
			}
			if mode == "IdP_only" {
				m.ResourceScopes = m.ResourceScopes[1:]
			}
			if mode == "deny_IdP" {
				m.DenyResources = []ResourceScope{{Origin: a.IdPOrigin, PathPrefix: "/"}}
			}
			if mode == "deny_verify" {
				m.DenyResources = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/account"}}
			}
			store.mandates[m.ID] = m
			reader.current.MandateID = m.ID
			reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			call.ToolName = browsersso.InspectTool
			call.CredentialRefs = nil
			if mode == "wrong_RP" {
				page.bound.URL = "https://rp.example/other"
			}
			if mode == "foreign_run" {
				call.RunId = "foreign"
			}
			if mode == "no_task" {
				reader.current.TaskID = ""
			}
			if mode == "no_mandate" {
				reader.current.Authority = nil
			}
			if mode == "credentials" {
				call.CredentialRefs = append(call.CredentialRefs, nil)
			}
			raw, _ := json.Marshal(a)
			call.ArgsJson = string(raw)
			contract := browsersso.InspectContract()
			if mode == "changed_contract" {
				contract = proto.Clone(contract).(*toolbrokerv1.ToolContract)
				contract.ContractRevision = "2"
			}
			p, err := s.Prepare(t.Context(), call, contract)
			allowed := mode == "allowed" || mode == "revoked_after_prepare" || mode == "changed_epoch"
			if !allowed {
				if err == nil {
					t.Fatal("inspection gained unsupported authority")
				}
				return
			}
			if err != nil || p.RequiresApproval || p.Binding.BrowserSSOInspection == nil || p.Binding.BrowserPage.Ref != a.RPPage || p.Binding.Effect != EffectRead {
				t.Fatal("bounded read prepare", err)
			}
			policy, err := ResolveEffectPolicy(p.Binding, contract, call)
			if err != nil || policy.RetrySafety != "read_only" || policy.VerificationMethod != "read_result" {
				t.Fatal("inspection borrowed write policy", err)
			}
			g, err := s.Authorize(t.Context(), call, contract, p.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "revoked_after_prepare" {
				m.Status = "revoked"
				store.mandates[m.ID] = m
			}
			if mode == "changed_epoch" {
				page.bound.Epoch++
			}
			_, err = s.Verify(t.Context(), call, contract, g.ID)
			if (err == nil) != (mode == "allowed") {
				t.Fatal("stale inspection grant", err)
			}
			if strings.Contains(p.Binding.Resource, "credential") {
				t.Fatal("inspection acquired secret scope")
			}
		})
	}
}
