package actionauth

import "github.com/butler/butler/internal/toolcatalog"

// Catalog selection is a fixed local, credential-free read. It can inspect only
// the current run's scoped catalog, including a child's narrower permissions.
func standardCatalogAction(b ActionBinding) bool {
	c := toolcatalog.Contract(b.ToolName)
	if c == nil || b.Resource != "tool:"+b.ToolName || b.Effect != EffectRead || len(b.CredentialRefs) > 0 {
		return false
	}
	h, err := ContractHash(c)
	return err == nil && h == b.ContractHash
}
