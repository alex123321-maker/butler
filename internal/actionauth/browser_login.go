package actionauth

import (
	"encoding/hex"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
)

// Value-free, immutable evidence correlated with the executing policy by PG.
type LoginEvidence struct {
	ScopeDigest   string            `json:"scope_digest"`
	LandingDigest string            `json:"landing_digest"`
	Browser       string            `json:"browser_session_id"`
	Version       uint64            `json:"observation_version"`
	Proof         browserauth.Proof `json:"authentication"`
}

func loginEvidence(p EffectPolicy, raw string) *LoginEvidence {
	var r struct {
		Browser string            `json:"browser_session_id"`
		Version uint64            `json:"observation_version"`
		URL     string            `json:"final_url"`
		Proof   browserauth.Proof `json:"authentication"`
	}
	if p.Login == nil || json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	scope, _ := json.Marshal(p.Login.Scope)
	return &LoginEvidence{ScopeDigest: digest(string(scope)), LandingDigest: digest(r.URL), Browser: r.Browser, Version: r.Version, Proof: r.Proof}
}
func validLoginEvidence(p EffectPolicy, b ActionBinding, e *LoginEvidence) bool {
	if p.Login == nil || e == nil || b.ToolName != browserauth.Tool || b.ContractRevision != browserauth.Revision || b.Effect != EffectCommit || !vaultRequestBinding(b) || p.Login.Scope.Validate() != nil || e.Browser != p.Login.Browser || e.Version <= p.Login.Version || e.Proof.Protocol != browserauth.Verification || !e.Proof.SubmittedOnce || !e.Proof.AuthenticatedRead || !e.Proof.AnonymousChallenge {
		return false
	}
	scope, _ := json.Marshal(p.Login.Scope)
	_, err := hex.DecodeString(e.LandingDigest)
	return e.ScopeDigest == digest(string(scope)) && len(e.LandingDigest) == 64 && err == nil && e.LandingDigest != digest(p.Login.ExpectedURL)
}

func loginVerdict(p EffectPolicy, b ActionBinding, raw string) EffectState {
	if p.Login == nil || b.ToolName != browserauth.Tool || b.ContractRevision != browserauth.Revision || b.Effect != EffectCommit || !vaultRequestBinding(b) || len(raw) > 512*1024 {
		return StateUnknown
	}
	if _, e := canonicalJSON(raw); e != nil {
		return StateUnknown
	}
	var r struct {
		OK        bool               `json:"ok"`
		Session   string             `json:"browser_session_id"`
		URL       string             `json:"final_url"`
		Version   uint64             `json:"observation_version"`
		Network   string             `json:"network_state"`
		Auth      *browserauth.Proof `json:"authentication"`
		Error     json.RawMessage    `json:"error"`
		Truncated bool               `json:"truncated"`
	}
	if json.Unmarshal([]byte(raw), &r) != nil || !r.OK || r.Session != p.Login.Browser || r.Version <= p.Login.Version || r.Network != "frozen" || r.Truncated || (len(r.Error) > 0 && string(r.Error) != "null") || browserauth.Origin(r.URL) != browserauth.Origin(p.Login.TargetURL) || r.URL == p.Login.ExpectedURL || r.Auth == nil || r.Auth.Protocol != browserauth.Verification || !r.Auth.SubmittedOnce || !r.Auth.AuthenticatedRead || !r.Auth.AnonymousChallenge {
		return StateUnknown
	}
	var fields map[string]json.RawMessage
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil || json.Unmarshal(root["authentication"], &fields) != nil || len(fields) != 4 {
		return StateUnknown
	}
	return StateConfirmed
}
