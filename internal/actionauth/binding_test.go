package actionauth

import (
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"testing"
)

func fixtureBinding(t *testing.T, args string, modify func(*toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, *ContextSnapshot)) ActionBinding {
	t.Helper()
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "http.request", ArgsJson: args}
	contract := &toolbrokerv1.ToolContract{ToolName: "http.request", Status: "active", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{"type":"object"}`, RuntimeTarget: "http:9090"}
	context := ContextSnapshot{RunID: "run", OwnerID: "owner", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}
	if modify != nil {
		modify(call, contract, &context)
	}
	binding, _, err := BuildBinding("action", call, contract, context)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
func TestActionFingerprintNormalizesObjectOrdering(t *testing.T) {
	a := fixtureBinding(t, `{"url":"https://example.com","method":"POST","body":{"b":2,"a":1}}`, nil)
	b := fixtureBinding(t, `{ "body": {"a":1,"b":2}, "method":"POST", "url":"https://example.com"}`, nil)
	if a.ActionHash != b.ActionHash {
		t.Fatal("object ordering changed normalized fingerprint")
	}
}
func TestApprovalBindingRejectsChangedIntent(t *testing.T) {
	baseline := fixtureBinding(t, `{"url":"https://example.com","method":"GET"}`, nil)
	for _, modify := range []func(*toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, *ContextSnapshot){
		func(call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ *ContextSnapshot) {
			call.ArgsJson = `{"url":"https://example.com","method":"POST"}`
		},
		func(_ *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, _ *ContextSnapshot) {
			contract.RuntimeTarget = "other:9090"
		},
		func(_ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, context *ContextSnapshot) {
			context.TaskRevision++
		},
		func(_ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, context *ContextSnapshot) {
			context.ConstraintsRevision++
		},
		func(_ *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, context *ContextSnapshot) {
			context.OwnerID = "other"
		},
	} {
		changed := fixtureBinding(t, `{"url":"https://example.com","method":"GET"}`, modify)
		if changed.ActionHash == baseline.ActionHash {
			t.Fatal("changed intent retained approval fingerprint")
		}
	}
}
func TestCredentialRefsMustMatchExactArgs(t *testing.T) {
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "browser.fill", ArgsJson: `{"value":{"type":"credential_ref","alias":"login","field":"password"}}`, CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "other", Field: "password"}}}
	contract := &toolbrokerv1.ToolContract{ToolName: "browser.fill", Status: "active", InputSchemaJson: `{"type":"object"}`, SupportsCredentialRefs: true}
	if _, _, err := BuildBinding("action", call, contract, ContextSnapshot{RunID: "run", OwnerID: "owner"}); err == nil {
		t.Fatal("credential substitution accepted")
	}
}
func TestOpaqueBrowserEffectsRequireExplicitAuthority(t *testing.T) {
	for _, tool := range []string{"browser.fill", "browser.type", "browser.click", "single_tab.click", "single_tab.press_keys", "plugin.operation"} {
		if classifyEffect(tool, nil) != EffectUnknown {
			t.Fatalf("%s opaque site effects treated as known safe", tool)
		}
	}
	if classifyEffect("http.request", map[string]any{"method": "POST"}) != EffectCommit || classifyEffect("browser.snapshot", nil) != EffectRead {
		t.Fatal("read and commit effects collapsed")
	}
}
