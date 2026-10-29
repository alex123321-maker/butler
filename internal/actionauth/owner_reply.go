package actionauth

import "github.com/butler/butler/internal/communicationcontrol"

// Publishing a response to the current owner's operator-pinned channels is a
// normal mode2/3 capability. It does not authorize external recipients or bypass
// source/export checks. OwnerCommunication remains the narrower unknown-effect
// budget exception. A fresh reply after stopped-attempt isolation requires its
// separate durable Input/intent/admission proof; this rule alone cannot allow it.
func standardOwnerReply(b ActionBinding, c ContextSnapshot) bool {
	if c.AutonomyMode < 3 || c.Delegation != nil || !c.CommunicationReady || c.CommunicationMode != "explicit" || !contains(c.CommunicationMethods, b.ToolName) || len(c.InputClaimHash) != 64 || len(b.CommunicationDestinationsHash) != 64 || len(b.CredentialRefs) != 0 || b.Effect != EffectPrepare || b.Resource != "tool:"+b.ToolName || CheckDenyOverrides(b, c.DenyTools, c.DenyResources) != nil {
		return false
	}
	return ownerReplyResource(b)
}

// This fixed local resource is usable by an owner-goal mandate only if the
// method is explicitly listed and prepare is allowed. Live communication
// admission independently pins the current Input and operator destinations.
func ownerReplyResource(b ActionBinding) bool {
	if b.Effect != EffectPrepare || b.Resource != "tool:"+b.ToolName || len(b.CredentialRefs) != 0 || len(b.CommunicationDestinationsHash) != 64 {
		return false
	}
	contract := communicationcontrol.Contract(b.ToolName)
	if contract == nil {
		return false
	}
	hash, err := ContractHash(contract)
	return err == nil && hash == b.ContractHash
}
