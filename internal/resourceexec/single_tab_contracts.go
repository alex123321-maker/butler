package resourceexec

import (
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

// SingleTabContracts is the fixed guarded wire contract shared by Broker and
// the installed native runtime. It exposes no host, token or native tab ref.
func SingleTabContracts(target string) []*toolbrokerv1.ToolContract {
	return []*toolbrokerv1.ToolContract{
		{ToolName: "single_tab.status", ToolClass: "single_tab", ContractRevision: SingleTabContractRevision, RuntimeTarget: target, Status: "enabled", RiskLevel: "low", Description: "Observe the explicitly selected tab. The observation is invalidated by page changes or another executor.", InputSchemaJson: `{"type":"object","required":["session_id"],"properties":{"session_id":{"type":"string","minLength":1,"maxLength":128}},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object","required":["ok","observation_id","observation_version"],"properties":{"ok":{"type":"boolean"},"observation_id":{"type":"string"},"observation_version":{"type":"integer"}},"additionalProperties":false}`, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}},
		{ToolName: "single_tab.click", ToolClass: "single_tab", ContractRevision: SingleTabContractRevision, RuntimeTarget: target, Status: "enabled", RiskLevel: "high", Description: "Click once in the explicitly selected tab using its current observation. Local completion does not verify an external effect.", InputSchemaJson: `{"type":"object","required":["session_id","observation_id","selector"],"properties":{"session_id":{"type":"string","minLength":1,"maxLength":128},"observation_id":{"type":"string","minLength":36,"maxLength":36},"selector":{"type":"string","minLength":1,"maxLength":1024}},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object","required":["ok","observation_id","observation_version"],"properties":{"ok":{"type":"boolean"},"observation_id":{"type":"string"},"observation_version":{"type":"integer"}},"additionalProperties":false}`, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "none"}},
	}
}

func ExactSingleTabContract(c *toolbrokerv1.ToolContract) bool {
	if c == nil || c.GetRuntimeTarget() == "" {
		return false
	}
	for _, expected := range SingleTabContracts(c.GetRuntimeTarget()) {
		if proto.Equal(c, expected) {
			return true
		}
	}
	return false
}

func SingleTabReadinessService(target, host string) string {
	hashes := ""
	for _, c := range SingleTabContracts(target) {
		h, err := actionauth.ContractHash(c)
		if err != nil {
			return ""
		}
		hashes += h
	}
	return "butler.guarded-tab." + hash(host+"\x00"+hashes)
}
