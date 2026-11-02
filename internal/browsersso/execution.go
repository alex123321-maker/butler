package browsersso

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/butler/butler/internal/browserauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/jsonstrict"
)

const Tool = "browser.sso_login"
const RequestTool = "credentials.request_sso_login"
const Revision = "1"
const Verification = "browser_sso_login_v1"

type Args struct {
	Scope
	Digest      string          `json:"scope_digest"`
	UsernameRef browserauth.Ref `json:"username"`
	PasswordRef browserauth.Ref `json:"password"`
}

func Parse(raw string) (Args, error) {
	var a Args
	if len(raw) > 32768 || jsonstrict.Decode([]byte(raw), &a) != nil || a.Scope.Validate() != nil || !digest(a.Digest) || a.UsernameRef.Type != "credential_ref" || a.PasswordRef.Type != "credential_ref" || !strings.HasPrefix(a.UsernameRef.Alias, "ssoq_") || !digest(strings.TrimPrefix(a.UsernameRef.Alias, "ssoq_")) || a.UsernameRef.Alias != a.PasswordRef.Alias || a.UsernameRef.Field != "username" || a.PasswordRef.Field != "password" {
		return Args{}, ErrScope
	}
	return a, nil
}
func digest(value string) bool {
	raw, e := hex.DecodeString(value)
	return e == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

// Targets is an all-of resource requirement, never a list of alternatives.
func (s Scope) Targets() []string { return []string{s.RPURL, s.IdPURL, s.TargetURL, s.VerifyURL} }

const ScopeSchema = `{"type":"object","required":["browser_session_id","rp_page","rp_url","trigger_selector","idp_url","target_url","form_selector","username_selector","password_selector","verify_url"],"properties":{"browser_session_id":{"type":"string"},"rp_page":{"type":"object","required":["page_id","document_id","observation_version"],"properties":{"page_id":{"type":"string"},"document_id":{"type":"string"},"observation_version":{"type":"integer","minimum":1}},"additionalProperties":false},"rp_url":{"type":"string"},"trigger_selector":{"type":"string"},"idp_url":{"type":"string"},"target_url":{"type":"string"},"form_selector":{"type":"string"},"username_selector":{"type":"string"},"password_selector":{"type":"string"},"verify_url":{"type":"string"}},"additionalProperties":false}`

func Contract() *toolbrokerv1.ToolContract {
	var schema map[string]any
	_ = json.Unmarshal([]byte(ScopeSchema), &schema)
	properties := schema["properties"].(map[string]any)
	ref := func(field string) map[string]any {
		return map[string]any{"type": "object", "required": []string{"type", "alias", "field"}, "properties": map[string]any{"type": map[string]any{"type": "string", "enum": []string{"credential_ref"}}, "alias": map[string]any{"type": "string"}, "field": map[string]any{"type": "string", "enum": []string{field}}}, "additionalProperties": false}
	}
	properties["username"], properties["password"], properties["scope_digest"] = ref("username"), ref("password"), map[string]any{"type": "string"}
	schema["required"] = append(schema["required"].([]any), "username", "password", "scope_digest")
	raw, _ := json.Marshal(schema)
	return &toolbrokerv1.ToolContract{ToolName: Tool, ContractRevision: Revision, ToolClass: "browser", RuntimeTarget: "tool-browser:9090", Description: "Perform one consented popup SSO login from an exact RP page through the named IdP and verify protected RP access. Requires the exact approved credentials.request_sso_login reference and scope digest. One native form POST, no automatic retry; no generic click/POST permission.", InputSchemaJson: string(raw), OutputSchemaJson: `{"type":"object"}`, RiskLevel: "high", SupportsCredentialRefs: true, RequiresApproval: true, Status: "active", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: Verification}}
}
