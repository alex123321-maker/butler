package browserauth

import toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"

const RefSchema = `{"type":"object","required":["type","alias","field"],"properties":{"type":{"type":"string","enum":["credential_ref"]},"alias":{"type":"string"},"field":{"type":"string","enum":["username","password"]}},"additionalProperties":false}`
const InputSchema = `{"type":"object","required":["browser_session_id","expected_url","target_url","observation_version","form_selector","username_selector","password_selector","username","password"],"properties":{"browser_session_id":{"type":"string"},"expected_url":{"type":"string"},"target_url":{"type":"string"},"observation_version":{"type":"integer","minimum":1},"form_selector":{"type":"string"},"username_selector":{"type":"string"},"password_selector":{"type":"string"},"username":` + RefSchema + `,"password":` + RefSchema + `},"additionalProperties":false}`

func Contract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: Tool, ContractRevision: Revision, ToolClass: "browser", Description: "Fill and submit one ordinary same-origin login form and verify authenticated access. First inspect forms and obtain credentials.request_login consent for this exact scope. Use that approved request reference for both username/password. Submits at most once, with no automatic retry. Unsupported/ambiguous login remains unknown. Generic click/POST approval is unchanged.", InputSchemaJson: InputSchema, OutputSchemaJson: `{"type":"object","properties":{"ok":{"type":"boolean"},"text":{"type":"string"},"final_url":{"type":"string"},"browser_session_id":{"type":"string"},"observation_version":{"type":"integer"},"network_state":{"type":"string"},"authentication":{"type":"object","properties":{"protocol":{"type":"string"},"submitted_once":{"type":"boolean"},"authenticated_read":{"type":"boolean"},"anonymous_challenge":{"type":"boolean"}}}}}`, RuntimeTarget: "tool-browser:9090", RiskLevel: "high", SupportsCredentialRefs: true, RequiresApproval: true, Status: "active", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: Verification}}
}
func ValidContract(c *toolbrokerv1.ToolContract) bool {
	return c != nil && c.ToolName == Tool && c.ContractRevision == Revision && c.SupportsCredentialRefs && c.GetEffectContract().GetRetrySafety() == "unsafe" && c.GetEffectContract().GetVerificationMethod() == Verification && c.GetEffectContract().GetIdempotencyHeader() == "" && c.GetEffectContract().GetScopeOrigin() == "" && c.GetEffectContract().GetVerificationUrl() == ""
}
