package actionauth

import "github.com/butler/butler/internal/delegation"

func delegationAllows(c *delegation.Contract, b ActionBinding) bool {
	if standardCatalogAction(b) {
		return true
	}
	refs := make([]delegation.CredentialRef, len(b.CredentialRefs))
	for i, r := range b.CredentialRefs {
		refs[i] = delegation.CredentialRef{Alias: r.Alias, Field: r.Field}
	}
	return delegation.Allows(c, b.ToolName, b.ContractHash, b.Resource, string(b.Effect), refs)
}
