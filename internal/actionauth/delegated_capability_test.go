package actionauth

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type delegatedCapabilityFixture struct {
	mutate func(*ActionBinding)
}

func (f delegatedCapabilityFixture) BindAction(_ context.Context, _ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, b ActionBinding, _ ContextSnapshot) (ActionBinding, error) {
	if f.mutate != nil {
		f.mutate(&b)
	}
	return b, nil
}

func TestReviewedCapabilityBindingCannotWidenDelegatedPermission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ActionBinding)
	}{
		{"same binding", nil},
		{"different resource", func(b *ActionBinding) { b.Resource = "tool:converters.activate" }},
		{"different effect", func(b *ActionBinding) { b.Effect = EffectCommit }},
		{"new credential reference", func(b *ActionBinding) { b.CredentialRefs = []CredentialRef{{Alias: "ungranted", Field: "token"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, store, reader, inspection, call, contract := fixture(t)
			call.ToolName, call.ArgsJson = "converters.discover", `{}`
			contract.ToolName = call.ToolName
			b, _, err := BuildBinding(StableActionID(call.RunId, call.ToolCallId), call, contract, reader.current)
			if err != nil {
				t.Fatal(err)
			}
			reader.current.Delegation = &delegation.Contract{Permissions: []delegation.Permission{{Tool: b.ToolName, ContractHash: b.ContractHash, Resources: []string{b.Resource}, Effects: []string{string(b.Effect)}}}}
			s.SetCapabilities(delegatedCapabilityFixture{mutate: tc.mutate})
			_, err = s.Prepare(context.Background(), call, contract)
			if tc.mutate == nil {
				if err != nil || inspection.calls != 0 || len(store.intents) != 1 {
					t.Fatal("exact permitted capability was refused", err)
				}
			} else if err == nil || inspection.calls != 0 || len(store.intents) != 0 {
				t.Fatal("rebound capability exceeded the child contract before credential inspection", err, inspection.calls)
			}
		})
	}
}
