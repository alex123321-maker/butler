package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/credentialcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type ssoInspector struct {
	*inspector
	approved bool
}

func (i *ssoInspector) ApprovedLoginUse(_ context.Context, c *toolbrokerv1.ToolCall) bool {
	return i.approved && c.ToolName == browsersso.Tool
}

func TestSSOConsentDoesNotReplaceLiveActionAuthority(t *testing.T) {
	for _, mode := range []string{"allowed", "no_consent", "RP_only", "IdP_only", "deny_IdP", "revoke", "replace_Input", "change_RP_epoch"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, inspect, call, _ := fixture(t)
			inspect.hash = strings.Repeat("c", 64)
			s.credentials = &ssoInspector{inspect, mode != "no_consent"}
			reader.current.InputClaimHash = strings.Repeat("a", 64)
			scope := ssoTestScope()
			hash, _ := scope.Digest(2, 1)
			alias := "ssoq_" + strings.Repeat("e", 64)
			a := browsersso.Args{Scope: scope, Digest: hash, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "password"}}
			raw, _ := json.Marshal(a)
			call.ToolName = browsersso.Tool
			call.ArgsJson = string(raw)
			call.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: alias, Field: "username"}, {Type: "credential_ref", Alias: alias, Field: "password"}}
			page := &pageScopeFixture{bound: browserpage.Binding{Ref: scope.RPPage, URL: scope.RPURL, Epoch: 2, RuntimeVersion: 5}}
			s.SetBrowserScopes(page)
			m := testOwnerMandate(s.now())
			m.OwnerID = reader.current.OwnerID
			m.Tools = []string{browsersso.Tool}
			m.Effects = []Effect{EffectCommit}
			m.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://idp.example", PathPrefix: "/"}}
			m.CredentialRefs = nil
			if mode == "RP_only" {
				m.ResourceScopes = m.ResourceScopes[:1]
			}
			if mode == "IdP_only" {
				m.ResourceScopes = m.ResourceScopes[1:]
			}
			if mode == "deny_IdP" {
				m.DenyResources = []ResourceScope{{Origin: "https://idp.example", PathPrefix: "/login"}}
			}
			store.mandates[m.ID] = m
			reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			p, e := s.Prepare(t.Context(), call, browsersso.Contract())
			if mode == "RP_only" || mode == "IdP_only" || mode == "deny_IdP" {
				if e == nil {
					t.Fatal("owner consent replaced destination authority")
				}
				return
			}
			if e != nil {
				t.Fatal("SSO prepare", e)
			}
			if mode == "no_consent" {
				if !p.RequiresApproval {
					t.Fatal("unapproved SSO waived action approval")
				}
				return
			}
			if p.RequiresApproval {
				t.Fatal("second POST approval required")
			}
			g, e := s.Authorize(t.Context(), call, browsersso.Contract(), p.Binding.ActionID, "", "")
			if e != nil || g.ApprovalID != "" {
				t.Fatal("protected SSO action authorization", e)
			}
			switch mode {
			case "revoke":
				m.Status = "revoked"
				store.mandates[m.ID] = m
			case "replace_Input":
				reader.current.InputClaimHash = strings.Repeat("f", 64)
			case "change_RP_epoch":
				page.bound.Epoch++
			}
			_, e = s.Verify(t.Context(), call, browsersso.Contract(), g.ID)
			if (e == nil) != (mode == "allowed") {
				t.Fatal("stale SSO action grant classification", e)
			}
		})
	}
}

func TestSSOEffectRequiresExactRPAccessProof(t *testing.T) {
	s := ssoTestScope()
	a := browsersso.Args{Scope: s, Digest: strings.Repeat("d", 64)}
	alias := "ssoq_" + strings.Repeat("e", 64)
	b := ActionBinding{ToolName: browsersso.Tool, ContractRevision: browsersso.Revision, Effect: EffectCommit, BrowserSSO: &s, BrowserPage: &browserpage.Binding{Ref: s.RPPage, URL: s.RPURL, RuntimeVersion: 1}, CredentialMetadataHash: strings.Repeat("f", 64), CredentialRefs: []CredentialRef{{Alias: alias, Field: "username"}, {Alias: alias, Field: "password"}}}
	p := EffectPolicy{SSO: &a}
	post := s.RPPage
	post.Version++
	post.DocumentID = strings.Repeat("9", 32)
	makeResult := func(page browserpage.Ref, protocol string, anonymous bool, url string) string {
		raw, _ := json.Marshal(map[string]any{"ok": true, "browser_session_id": s.Browser, "final_url": url, "page": page, "network_state": "frozen", "authentication": browserauth.Proof{Protocol: protocol, SubmittedOnce: true, AuthenticatedRead: true, AnonymousChallenge: anonymous}})
		return string(raw)
	}
	good := makeResult(post, browsersso.Verification, true, s.VerifyURL)
	if !validSSOEvidence(p, b, ssoEvidence(p, good)) {
		t.Fatal("exact RP proof rejected")
	}
	foreign := post
	foreign.ID = strings.Repeat("8", 32)
	for _, raw := range []string{makeResult(post, browserauth.Verification, true, s.VerifyURL), makeResult(post, browsersso.Verification, false, s.VerifyURL), makeResult(foreign, browsersso.Verification, true, s.VerifyURL), makeResult(s.RPPage, browsersso.Verification, true, s.VerifyURL), makeResult(post, browsersso.Verification, true, s.IdPURL), strings.Replace(good, `"authentication":{`, `"authentication":{"extra":true,`, 1), strings.Replace(good, `"ok":true`, `"ok":true,"ok":true`, 1)} {
		if validSSOEvidence(p, b, ssoEvidence(p, raw)) {
			t.Fatal("unverified or foreign RP proof confirmed")
		}
	}
}

func ssoTestScope() browsersso.Scope {
	return browsersso.Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", IdPURL: "https://idp.example/authorize", TargetURL: "https://idp.example/login", VerifyURL: "https://rp.example/account", Trigger: "#sso", Form: "form", Username: "input[name=username]", Password: "input[type=password]"}
}

func TestSSOAuthorityRequiresEveryDestination(t *testing.T) {
	now := time.Now()
	m := testOwnerMandate(now)
	scope := ssoTestScope()
	m.Tools = append(m.Tools, browsersso.Tool)
	m.Effects = append(m.Effects, EffectPrepare)
	m.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://idp.example", PathPrefix: "/"}}
	for _, tool := range []string{browsersso.Tool, browsersso.RequestTool} {
		b := ActionBinding{ToolName: tool, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose, Effect: EffectCommit, BrowserSSO: &scope, Resource: "browser:" + scope.Browser + ":" + scope.TargetURL}
		if tool == browsersso.RequestTool {
			b.Resource = "tool:" + tool
			b.Effect = EffectPrepare
			b.ContractHash, _ = ContractHash(credentialcontrol.Contract(tool))
		}
		if CheckAssignedAction(m, b, now) != nil {
			t.Fatal("both-origin authority rejected", tool)
		}
		for _, keep := range []int{0, 1} {
			limited := m
			limited.ResourceScopes = []ResourceScope{m.ResourceScopes[keep]}
			if CheckAssignedAction(limited, b, now) == nil {
				t.Fatal("one origin authorized both", tool)
			}
		}
		limited := m
		limited.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://idp.example", PathPrefix: "/authorize"}}
		if CheckAssignedAction(limited, b, now) == nil {
			t.Fatal("IdP read path authorized POST target")
		}
		for _, origin := range []string{"https://rp.example", "https://idp.example"} {
			denial := ResourceScope{Origin: origin, PathPrefix: "/"}
			if CheckDenyOverrides(b, nil, []ResourceScope{denial}) == nil {
				t.Fatal("origin denial ignored")
			}
			blocked := m
			blocked.DenyResources = []ResourceScope{denial}
			if CheckAssignedAction(blocked, b, now) == nil {
				t.Fatal("mandate denial ignored")
			}
		}
		b.ToolName = "browser.click"
		if CheckDenyOverrides(b, nil, nil) == nil {
			t.Fatal("SSO scope attached to generic mutation")
		}
	}
}
