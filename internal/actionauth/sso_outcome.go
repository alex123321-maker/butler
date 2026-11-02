package actionauth

import "github.com/butler/butler/internal/browsersso"

// Historical factual review never resolves a field, resumes a failed Input,
// refunds a claim or grants a retry. Require the original closed SSO policy and
// exact RP/page/arguments before offering the existing owner-attestation path.
func historicalSSOSource(e EffectRecord, b ActionBinding, raw string) bool {
	if e.Policy.SSO == nil || e.Policy.RetrySafety != "unsafe" || e.Policy.VerificationMethod != browsersso.Verification || b.ToolName != browsersso.Tool || b.ContractRevision != browsersso.Revision || b.Effect != EffectCommit || !vaultRequestBinding(b) || b.BrowserPage == nil || !b.BrowserPage.Valid(true) {
		return false
	}
	a, err := browsersso.Parse(raw)
	if err != nil {
		return false
	}
	h, err := ArgumentsHash(mustOwnerJSON(e.Policy.SSO))
	if err != nil || h != b.ArgsHash {
		return false
	}
	contractHash, err := ContractHash(browsersso.Contract())
	return err == nil && b.ContractHash == contractHash && a.Scope == *b.BrowserSSO && a.Scope == e.Policy.SSO.Scope && b.BrowserPage.Ref == a.RPPage && b.BrowserPage.URL == a.RPURL && b.Resource == "browser:"+a.Browser+":"+a.TargetURL
}
