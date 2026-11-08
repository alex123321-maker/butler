package actionauth

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/browserspa"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

func TestSPARequiresExactCurrentOwnerGoalRead(t *testing.T) {
	for _, mode := range []string{"allowed", "no_task", "no_goal", "no_input", "credentials", "changed_contract", "foreign_origin", "denied_path", "no_read", "revoked_after_prepare", "changed_args"} {
		t.Run(mode, func(t *testing.T) {
			s, store, reader, _, call, _ := fixture(t)
			reader.current.AutonomyMode = 3
			reader.current.InputClaimHash = InputClaimHash("spa-input", "spa-claim")
			call.AutonomyMode = 3
			m := testOwnerMandate(s.now())
			m.OwnerID = reader.current.OwnerID
			m.Tools = []string{browserspa.Tool}
			m.Effects = []Effect{EffectRead}
			m.CredentialRefs = nil
			m.ResourceScopes = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/catalog"}}
			reader.current.MandateID = m.ID
			reader.current.Authority = &Assignment{TaskID: reader.current.TaskID, OwnerID: m.OwnerID, GoalID: m.GoalID, GoalRevision: m.GoalRevision, MandateID: m.ID, MandateRevision: m.Revision, Purpose: m.Purpose}
			a := browserspa.Args{URL: "https://rp.example/catalog/", Filter: &browserspa.Filter{Selector: "input[name=filter]", Value: "books"}}
			call.ToolName = browserspa.Tool
			call.CredentialRefs = nil
			c := browserspa.Contract()
			switch mode {
			case "no_task":
				reader.current.TaskID = ""
			case "no_goal":
				reader.current.Authority = nil
			case "no_input":
				reader.current.InputClaimHash = ""
			case "credentials":
				call.CredentialRefs = append(call.CredentialRefs, nil)
			case "changed_contract":
				c = proto.Clone(c).(*toolbrokerv1.ToolContract)
				c.EffectContract.VerificationMethod = "none"
			case "foreign_origin":
				a.URL = "https://foreign.example/catalog/"
			case "denied_path":
				m.DenyResources = []ResourceScope{{Origin: "https://rp.example", PathPrefix: "/catalog"}}
			case "no_read":
				m.Effects = []Effect{EffectPrepare}
			}
			store.mandates[m.ID] = m
			raw, _ := json.Marshal(a)
			call.ArgsJson = string(raw)
			p, err := s.Prepare(t.Context(), call, c)
			allowed := mode == "allowed" || mode == "revoked_after_prepare" || mode == "changed_args"
			if !allowed {
				if err == nil {
					t.Fatal("unsupported SPA authority accepted")
				}
				return
			}
			if err != nil || p.RequiresApproval || p.Binding.Effect != EffectRead {
				t.Fatal("canonical scoped SPA read denied", err)
			}
			policy, err := ResolveEffectPolicy(p.Binding, c, call)
			if err != nil || policy.RetrySafety != "read_only" || policy.VerificationMethod != "read_result" {
				t.Fatal("SPA read policy drift", err)
			}
			grant, err := s.Authorize(t.Context(), call, c, p.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "revoked_after_prepare" {
				m.Status = "revoked"
				store.mandates[m.ID] = m
			}
			if mode == "changed_args" {
				a.Filter.Value = "other"
				raw, _ = json.Marshal(a)
				call.ArgsJson = string(raw)
			}
			_, err = s.Verify(t.Context(), call, c, grant.ID)
			if (err == nil) != (mode == "allowed") {
				t.Fatal("stale SPA grant accepted/valid read denied", err)
			}
		})
	}
}
