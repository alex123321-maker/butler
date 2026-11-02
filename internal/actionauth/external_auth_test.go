package actionauth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/externalauth"
)

func externalTestScope() externalauth.Scope {
	return externalauth.Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", Provider: "google", VerifyURL: "https://rp.example/auth/google/account"}
}
func externalTestMandate(now time.Time) Mandate {
	m := testOwnerMandate(now)
	m.Tools = []string{externalauth.RequestTool}
	m.CredentialRefs = nil
	m.Effects = []Effect{EffectPrepare, EffectCommit}
	m.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://accounts.google.com", PathPrefix: "/o/oauth2/v2/auth"}, {Origin: "https://oauth2.googleapis.com", PathPrefix: "/token"}, {Origin: "https://www.googleapis.com", PathPrefix: "/oauth2/v3/certs"}}
	return m
}
func TestExternalAuthAuthorityPinsEveryProviderAndRPResource(t *testing.T) {
	now := time.Now()
	m := externalTestMandate(now)
	scope := externalTestScope()
	b := ActionBinding{ToolName: externalauth.RequestTool, Resource: "tool:" + externalauth.RequestTool, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose, Effect: EffectPrepare, ExternalAuth: &scope}
	b.ContractHash, _ = ContractHash(credentialcontrol.Contract(b.ToolName))
	if CheckAssignedAction(m, b, now) != nil {
		t.Fatal("complete resource authority rejected")
	}
	for i, target := range scope.Targets() {
		t.Run(target, func(t *testing.T) {
			limited := m
			limited.ResourceScopes = append([]ResourceScope(nil), m.ResourceScopes...)
			remove := i
			if i == 1 {
				remove = 0
			} else if i > 1 {
				remove = i - 1
			}
			limited.ResourceScopes = append(limited.ResourceScopes[:remove], limited.ResourceScopes[remove+1:]...)
			if CheckAssignedAction(limited, b, now) == nil {
				t.Fatal("partial resource set authorized external login")
			}
			denial := ResourceScope{}
			for _, s := range m.ResourceScopes {
				if resourceWithin(s, target) {
					denial = s
					break
				}
			}
			if CheckDenyOverrides(b, nil, []ResourceScope{denial}) == nil {
				t.Fatal("destination denial ignored")
			}
		})
	}
	b.Effect = EffectCommit
	withoutTool := m
	withoutTool.Tools = nil
	if CheckAssignedAction(withoutTool, b, now) == nil {
		t.Fatal("standard request exemption granted external commit")
	}
	if CheckAssignedAction(m, b, now) != nil {
		t.Fatal("explicit commit authority rejected")
	}
	legacy := m
	legacy.ScopeKind = "task"
	if mandateAllows(legacy, b, now) {
		t.Fatal("legacy origin mandate authorized external login")
	}
	b.ToolName = "browser.click"
	if CheckDenyOverrides(b, nil, nil) == nil {
		t.Fatal("external scope upgraded a generic tool")
	}
}
func TestExternalAuthPrepareRequiresCurrentSourcePageAndOwnerAssignment(t *testing.T) {
	for _, mode := range []string{"current", "stale_page", "changed_url", "unassigned", "changed_epoch"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, _, call, _ := fixture(t)
			scope := externalTestScope()
			reader.current.InputClaimHash = strings.Repeat("d", 64)
			m := externalTestMandate(s.now())
			m.OwnerID = reader.current.OwnerID
			store.mandates[m.ID] = m
			reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			if mode == "unassigned" {
				reader.current.Authority = nil
			}
			page := &pageScopeFixture{bound: browserpage.Binding{Ref: scope.RPPage, URL: scope.RPURL, Epoch: 2, RuntimeVersion: 4}}
			if mode == "stale_page" {
				page.bound.Version++
			}
			if mode == "changed_url" {
				page.bound.URL = "https://rp.example/other"
			}
			s.SetBrowserScopes(page)
			a := externalauth.Request{RecordID: "cred_" + strings.Repeat("e", 64), Purpose: "One external login", Scope: scope}
			raw, _ := json.Marshal(a)
			call.ToolName = externalauth.RequestTool
			call.ArgsJson = string(raw)
			call.CredentialRefs = nil
			p, e := s.Prepare(context.Background(), call, credentialcontrol.Contract(call.ToolName))
			if mode == "stale_page" || mode == "changed_url" || mode == "unassigned" {
				if e == nil {
					t.Fatal("unbound external request prepared")
				}
				return
			}
			if e != nil || p.RequiresApproval {
				t.Fatal("local protected question required a generic approval", e)
			}
			g, e := s.Authorize(t.Context(), call, credentialcontrol.Contract(call.ToolName), p.Binding.ActionID, "", "")
			if e != nil {
				t.Fatal("current question authorization", e)
			}
			if mode == "changed_epoch" {
				page.bound.Epoch++
			}
			_, e = s.Verify(t.Context(), call, credentialcontrol.Contract(call.ToolName), g.ID)
			if (e == nil) != (mode == "current") {
				t.Fatal("source fence not enforced", e)
			}
		})
	}
}
