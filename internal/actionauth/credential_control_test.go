package actionauth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/credentialcontrol"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestCredentialInteractionBudgetIdentityRequiresCanonicalGrant(t *testing.T) {
	for _, name := range []string{"credentials.request_access", "credentials.request_create", "credentials.request_otp"} {
		c := credentialcontrol.Contract(name)
		hash, err := ContractHash(c)
		if err != nil {
			t.Fatal(err)
		}
		g := Grant{InputClaimHash: strings.Repeat("a", 64), Binding: ActionBinding{ToolName: name, Resource: "tool:" + name, ContractHash: hash, Effect: EffectPrepare}}
		if !toolBudgetIdentity(g, "effect").OwnerCredentialInteraction {
			t.Fatal("canonical protected request not qualified")
		}
		for _, scenario := range []string{"contract", "refs", "claim", "read", "browser"} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				changed := g
				switch scenario {
				case "contract":
					changed.Binding.ContractHash = "forged"
				case "refs":
					changed.Binding.CredentialRefs = []CredentialRef{{Alias: "record", Field: "password"}}
				case "claim":
					changed.InputClaimHash = ""
				case "read":
					changed.Binding.Effect = EffectRead
				case "browser":
					changed.Binding.ToolName = "browser.prepare_fill"
				}
				if toolBudgetIdentity(changed, "effect").OwnerCredentialInteraction {
					t.Fatal("unqualified grant widened unknown-effect exception")
				}
			})
		}
	}
}

func TestCredentialRequestDoesNotRequireApprovalToAsk(t *testing.T) {
	s, _, reader, _, call, _ := fixture(t)
	reader.current.InputClaimHash = strings.Repeat("a", 64)
	reader.current.AutonomyMode = 2
	call.AutonomyMode = commonv1.AutonomyMode_AUTONOMY_MODE_1
	call.ToolName = "credentials.request_create"
	call.ArgsJson = `{"browser_session_id":"browser","expected_url":"https://example.com/login","purpose":"Sign in","fields":["username","password"]}`
	contract := credentialcontrol.Contract(call.ToolName)
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil || p.RequiresApproval {
		t.Fatal("requesting a protected owner form requires another approval", err)
	}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
	if err != nil || g.ApprovalID != "" || g.InputClaimHash != reader.current.InputClaimHash {
		t.Fatal("local request grant invalid", err)
	}
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err != nil {
		t.Fatal("request grant verify", err)
	}
	reader.current.InputClaimHash = strings.Repeat("b", 64)
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("request survived input claim replacement")
	}
	reader.current.DenyTools = []string{call.ToolName}
	if _, err = s.Prepare(context.Background(), call, contract); err == nil {
		t.Fatal("request bypassed explicit denial")
	}
}

func TestCredentialRequestLocatorKeepsStandingMandateBoundaries(t *testing.T) {
	now := time.Now()
	m := testOwnerMandate(now)
	m.Tools = []string{"browser.prepare_fill"}
	m.Effects = []Effect{EffectPrepare}
	m.CredentialRefs = nil
	b := ActionBinding{OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose, ToolName: "browser.prepare_fill", ContractRevision: "2", Effect: EffectPrepare, Resource: "browser:context:https://example.invalid/test-service/login", CredentialMetadataHash: strings.Repeat("a", 64), CredentialRefs: []CredentialRef{{Alias: "creq_" + strings.Repeat("b", 64), Field: "password"}}}
	if CheckAssignedAction(m, b, now) != nil {
		t.Fatal("approved per-use locator required a predeclared random ID")
	}
	for name, mutate := range map[string]func(*ActionBinding){
		"unverified":   func(b *ActionBinding) { b.CredentialMetadataHash = "" },
		"static alias": func(b *ActionBinding) { b.CredentialRefs = []CredentialRef{{Alias: "some-account", Field: "password"}} },
		"token": func(b *ActionBinding) {
			b.CredentialRefs = []CredentialRef{{Alias: "creq_" + strings.Repeat("b", 64), Field: "token"}}
		},
		"different site": func(b *ActionBinding) { b.Resource = "browser:context:https://foreign.invalid/login" },
		"submit":         func(b *ActionBinding) { b.ToolName = "browser.click"; b.Effect = EffectUnknown },
		"old contract":   func(b *ActionBinding) { b.ContractRevision = "1" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := b
			mutate(&changed)
			if CheckAssignedAction(m, changed, now) == nil {
				t.Fatal("vault locator widened mandate scope")
			}
		})
	}
	m.DenyTools = []string{"browser.prepare_fill"}
	if CheckAssignedAction(m, b, now) == nil {
		t.Fatal("vault locator bypassed explicit deny")
	}
}

type vaultGrantInspector struct {
	*inspector
	active bool
}

func (i *vaultGrantInspector) ApprovedLocalUse(_ context.Context, call *toolbrokerv1.ToolCall) bool {
	return i.active && call.ToolName == "browser.prepare_fill"
}

func TestCredentialOwnerGrantIsLocalAndPinsInputClaim(t *testing.T) {
	s, store, reader, inspection, call, contract := fixture(t)
	checker := &vaultGrantInspector{inspector: inspection, active: true}
	s.credentials = checker
	reader.current.InputClaimHash = strings.Repeat("a", 64)
	inspection.hash = strings.Repeat("c", 64)
	m := testOwnerMandate(s.now())
	m.OwnerID = reader.current.OwnerID
	m.Tools = []string{"browser.prepare_fill"}
	m.ResourceScopes = []ResourceScope{{Origin: "https://example.com", PathPrefix: "/form"}}
	m.Effects, m.CredentialRefs = []Effect{EffectPrepare}, nil
	store.mandates[m.ID] = m
	reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
	alias := "creq_" + strings.Repeat("b", 64)
	call.ToolName = "browser.prepare_fill"
	call.ArgsJson = `{"browser_session_id":"context","expected_url":"https://example.com/form","observation_version":7,"selector":"#input","value":{"type":"credential_ref","alias":"` + alias + `","field":"password"}}`
	call.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: alias, Field: "password"}}
	contract.ToolName, contract.ContractRevision = call.ToolName, "2"
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_browser_preparation_v1"}
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil || p.RequiresApproval {
		t.Fatal("owner grant required a duplicate approval", err)
	}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
	if err != nil || g.InputClaimHash != reader.current.InputClaimHash {
		t.Fatal("owner preparation grant", err)
	}
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err != nil {
		t.Fatal("owner preparation verify", err)
	}
	reader.current.InputClaimHash = strings.Repeat("b", 64)
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("owner preparation survived Input replacement")
	}
	reader.current.InputClaimHash = g.InputClaimHash
	checker.active = false
	// In production revocation also changes metadata and prevents binding.
	inspection.err = credentialcontrol.ErrVault
	if _, err = s.Verify(context.Background(), call, contract, g.ID); err == nil {
		t.Fatal("owner preparation survived revocation")
	}
	inspection.err = nil
	checker.active = true
	reader.current.Authority = nil
	call.ToolCallId = "submit"
	call.ToolName, contract.ToolName = "browser.click", "browser.click"
	call.ArgsJson = `{"browser_session_id":"context","expected_url":"https://example.com/form","observation_version":8,"selector":"#submit"}`
	call.CredentialRefs = nil
	contract.EffectContract = nil
	p, err = s.Prepare(context.Background(), call, contract)
	if err != nil || !p.RequiresApproval {
		t.Fatal("owner credential grant authorized form submission", err)
	}
}
