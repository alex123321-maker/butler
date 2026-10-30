package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"strings"
	"testing"
)

type loginInspector struct {
	*inspector
	active bool
}

func (i *loginInspector) ApprovedLoginUse(_ context.Context, c *toolbrokerv1.ToolCall) bool {
	return i.active && c.ToolName == browserauth.Tool
}
func loginAuthorityFixture(t *testing.T) (*Service, *memoryStore, *contexts, *loginInspector, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract) {
	s, store, reader, i, call, _ := fixture(t)
	checker := &loginInspector{i, true}
	i.hash = strings.Repeat("c", 64)
	s.credentials = checker
	reader.current.InputClaimHash = strings.Repeat("a", 64)
	alias := "creq_" + strings.Repeat("b", 64)
	a := browserauth.Args{Scope: browserauth.Scope{Browser: "context", ExpectedURL: "https://example.com/login", TargetURL: "https://example.com/session", Form: "form", Username: "#u", Password: "#p"}, Version: 7, UsernameRef: browserauth.Ref{"credential_ref", alias, "username"}, PasswordRef: browserauth.Ref{"credential_ref", alias, "password"}}
	raw, _ := json.Marshal(a)
	call.ToolName = browserauth.Tool
	call.ArgsJson = string(raw)
	call.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: alias, Field: "username"}, {Type: "credential_ref", Alias: alias, Field: "password"}}
	m := testOwnerMandate(s.now())
	m.OwnerID = reader.current.OwnerID
	m.Tools = []string{browserauth.Tool}
	m.ResourceScopes = []ResourceScope{{Origin: "https://example.com", PathPrefix: "/"}}
	m.Effects = []Effect{EffectCommit}
	m.CredentialRefs = nil
	store.mandates[m.ID] = m
	reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
	return s, store, reader, checker, call, browserauth.Contract()
}
func TestCredentialLoginOneConsentKeepsAuthority(t *testing.T) {
	for _, mode := range []string{"allowed", "no_consent", "prepare_only", "tool_denied", "resource_denied", "revoked", "input_replaced"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, i, call, c := loginAuthorityFixture(t)
			if mode == "no_consent" {
				i.active = false
			}
			if mode == "prepare_only" || mode == "tool_denied" || mode == "resource_denied" || mode == "revoked" {
				m := store.mandates[reader.current.Authority.MandateID]
				switch mode {
				case "prepare_only":
					m.Effects = []Effect{EffectPrepare}
				case "tool_denied":
					m.DenyTools = []string{browserauth.Tool}
				case "resource_denied":
					m.DenyResources = []ResourceScope{{Origin: "https://example.com"}}
				case "revoked":
					m.Status = "revoked"
				}
				store.mandates[m.ID] = m
			}
			p, err := s.Prepare(context.Background(), call, c)
			if mode == "prepare_only" || mode == "tool_denied" || mode == "resource_denied" || mode == "revoked" {
				if err == nil {
					t.Fatal("login consent bypassed mandate")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "no_consent" {
				if !p.RequiresApproval {
					t.Fatal("login without consent waived approval")
				}
				return
			}
			if p.RequiresApproval {
				t.Fatal("duplicate submit approval")
			}
			g, err := s.Authorize(context.Background(), call, c, p.Binding.ActionID, "", "")
			if err != nil || g.ApprovalID != "" {
				t.Fatal("login grant", err)
			}
			if mode == "input_replaced" {
				reader.current.InputClaimHash = strings.Repeat("d", 64)
			}
			_, err = s.Verify(context.Background(), call, c, g.ID)
			if (err == nil) != (mode != "input_replaced") {
				t.Fatal("login claim fence", err)
			}
		})
	}
}
func TestLoginTypedVerdictAndPGEvidence(t *testing.T) {
	s, _, _, _, call, c := loginAuthorityFixture(t)
	p, err := s.Prepare(context.Background(), call, c)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ResolveEffectPolicy(p.Binding, c, call)
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"ok":true,"browser_session_id":"context","observation_version":8,"final_url":"https://example.com/account","network_state":"frozen","authentication":{"protocol":"browser_login_v1","submitted_once":true,"authenticated_read":true,"anonymous_challenge":true}}`
	for _, mode := range []string{"valid", "public", "wrong_context", "stale", "unfrozen", "login_page", "foreign", "extra_proof", "failed"} {
		t.Run(mode, func(t *testing.T) {
			raw := valid
			switch mode {
			case "public":
				raw = strings.Replace(raw, `"anonymous_challenge":true`, `"anonymous_challenge":false`, 1)
			case "wrong_context":
				raw = strings.Replace(raw, `"context"`, `"foreign"`, 1)
			case "stale":
				raw = strings.Replace(raw, `:8`, `:7`, 1)
			case "unfrozen":
				raw = strings.Replace(raw, `"frozen"`, `"live"`, 1)
			case "login_page":
				raw = strings.Replace(raw, `/account`, `/login`, 1)
			case "foreign":
				raw = strings.Replace(raw, `https://example.com/account`, `https://other.example/account`, 1)
			case "extra_proof":
				raw = strings.Replace(raw, `"protocol":`, `"untrusted":true,"protocol":`, 1)
			case "failed":
				raw = strings.Replace(raw, `"ok":true`, `"ok":false`, 1)
			}
			if (loginVerdict(policy, p.Binding, raw) == StateConfirmed) != (mode == "valid") {
				t.Fatal("unverified login confirmed")
			}
		})
	}
	e := loginEvidence(policy, valid)
	if !validLoginEvidence(policy, p.Binding, e) {
		t.Fatal("valid normalized evidence denied")
	}
	for _, mode := range []string{"scope", "version", "landing", "proof"} {
		changed := *e
		switch mode {
		case "scope":
			changed.ScopeDigest = "forged"
		case "version":
			changed.Version = 7
		case "landing":
			changed.LandingDigest = digest(policy.Login.ExpectedURL)
		case "proof":
			changed.Proof.AnonymousChallenge = false
		}
		if validLoginEvidence(policy, p.Binding, &changed) {
			t.Fatal("forged PG login evidence accepted", mode)
		}
	}
}
