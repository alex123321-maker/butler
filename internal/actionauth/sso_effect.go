package actionauth

import (
	"encoding/json"

	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
)

type SSOEvidence struct {
	ScopeDigest   string            `json:"scope_digest"`
	LandingDigest string            `json:"landing_digest"`
	Browser       string            `json:"browser_session_id"`
	Page          browserpage.Ref   `json:"page"`
	Proof         browserauth.Proof `json:"authentication"`
}

func ssoEvidence(p EffectPolicy, raw string) *SSOEvidence {
	if p.SSO == nil || len(raw) > 512*1024 {
		return nil
	}
	if _, e := canonicalJSON(raw); e != nil {
		return nil
	}
	var r struct {
		OK        bool               `json:"ok"`
		Browser   string             `json:"browser_session_id"`
		URL       string             `json:"final_url"`
		Page      browserpage.Ref    `json:"page"`
		Network   string             `json:"network_state"`
		Auth      *browserauth.Proof `json:"authentication"`
		Error     json.RawMessage    `json:"error"`
		Truncated bool               `json:"truncated"`
	}
	if json.Unmarshal([]byte(raw), &r) != nil || !r.OK || r.Network != "frozen" || r.Auth == nil || r.Truncated || len(r.Error) > 0 && string(r.Error) != "null" || r.URL != p.SSO.VerifyURL {
		return nil
	}
	var root, auth map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil || json.Unmarshal(root["authentication"], &auth) != nil || len(auth) != 4 {
		return nil
	}
	return &SSOEvidence{ScopeDigest: p.SSO.Digest, LandingDigest: digest(r.URL), Browser: r.Browser, Page: r.Page, Proof: *r.Auth}
}
func validSSOEvidence(p EffectPolicy, b ActionBinding, e *SSOEvidence) bool {
	return p.SSO != nil && e != nil && validSSOScope(b) && *b.BrowserSSO == p.SSO.Scope && b.BrowserPage != nil && b.BrowserPage.Ref == p.SSO.RPPage && b.ToolName == browsersso.Tool && b.ContractRevision == browsersso.Revision && b.Effect == EffectCommit && vaultRequestBinding(b) && e.Browser == p.SSO.Browser && e.Page.Valid() && e.Page.ID == p.SSO.RPPage.ID && e.Page.Version > p.SSO.RPPage.Version && e.ScopeDigest == p.SSO.Digest && e.LandingDigest == digest(p.SSO.VerifyURL) && e.Proof.Protocol == browsersso.Verification && e.Proof.SubmittedOnce && e.Proof.AuthenticatedRead && e.Proof.AnonymousChallenge
}
