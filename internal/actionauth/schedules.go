package actionauth

import "github.com/butler/butler/internal/schedulecontrol"
import "github.com/butler/butler/internal/toolcatalog"

// EffectiveMandateTools includes fixed kernel schedule controls for owner-goal
// work. These controls manage work in the current conversation; they do not add
// external runtime permissions. Callers must still check current authority,
// effects, explicit denials, composition and the original input claim.
// Legacy task-specific and delegated permissions retain their exact tool sets.
func EffectiveMandateTools(m Mandate) []string {
	tools := append([]string(nil), m.Tools...)
	for _, c := range toolcatalog.Contracts() {
		if !contains(tools, c.ToolName) {
			tools = append(tools, c.ToolName)
		}
	}
	if m.ScopeKind == "owner_goal" {
		for _, contract := range schedulecontrol.Contracts() {
			if !contains(tools, contract.ToolName) {
				tools = append(tools, contract.ToolName)
			}
		}
	}
	return tools
}

// Only the exact owning contract and its credential-free local resource may use
// the standard capability. A familiar name cannot widen a runtime/resource grant.
func standardScheduleAction(b ActionBinding) bool {
	contract := schedulecontrol.Contract(b.ToolName)
	if contract == nil || b.Resource != "tool:"+b.ToolName || len(b.CredentialRefs) != 0 {
		return false
	}
	effect := EffectPrepare
	if schedulecontrol.ReadOnly(b.ToolName) {
		effect = EffectRead
	}
	hash, err := ContractHash(contract)
	return err == nil && b.ContractHash == hash && b.Effect == effect
}
