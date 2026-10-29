package actionauth

import (
	"context"
	"encoding/hex"
	"github.com/butler/butler/internal/credentialcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"strings"
)

// Only fixed local search/request contracts may ask the owner without first
// asking permission to ask. They never release or authorize credential values.
func standardCredentialControl(b ActionBinding) bool {
	c := credentialcontrol.Contract(b.ToolName)
	if c == nil || b.Resource != "tool:"+b.ToolName || len(b.CredentialRefs) != 0 {
		return false
	}
	hash, err := ContractHash(c)
	effect := EffectPrepare
	if b.ToolName == "credentials.search" || b.ToolName == "credentials.request_status" {
		effect = EffectRead
	}
	return err == nil && hash == b.ContractHash && effect == b.Effect
}

// Scoped request references are reviewed at use time, so their random IDs
// cannot be listed in a standing mandate. This only satisfies its credential
// locator scope; tools, resources, effects and denials still apply. Broker must
// independently check the live owner request before resolving any value.
func vaultRequestBinding(b ActionBinding) bool {
	if b.Effect != EffectPrepare || (b.ToolName != "browser.prepare_fill" && b.ToolName != "browser.prepare_type") || (b.ContractRevision != "2" && b.ContractRevision != "3") || len(b.CredentialMetadataHash) != 64 || len(b.CredentialRefs) == 0 {
		return false
	}
	for _, ref := range b.CredentialRefs {
		if len(ref.Alias) != 69 || !strings.HasPrefix(ref.Alias, "creq_") || (ref.Field != "username" && ref.Field != "password" && ref.Field != "otp") {
			return false
		}
		if _, err := hex.DecodeString(ref.Alias[5:]); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) approvedCredentialPreparation(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) bool {
	if contract == nil || (contract.ToolName != "browser.prepare_fill" && contract.ToolName != "browser.prepare_type") || (contract.ContractRevision != "2" && contract.ContractRevision != "3") || !contract.SupportsCredentialRefs || contract.GetEffectContract().GetVerificationMethod() != "local_browser_preparation_v1" {
		return false
	}
	checker, ok := s.credentials.(interface {
		ApprovedLocalUse(context.Context, *toolbrokerv1.ToolCall) bool
	})
	return ok && checker.ApprovedLocalUse(ctx, call)
}
