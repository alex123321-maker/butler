package credentialcontrol

import (
	"errors"
	"github.com/butler/butler/internal/communicationcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

var ErrVault = errors.New("credential request unavailable or stale")

const ControlTarget = "broker://credential-vault-v1"

type ToolArgs struct {
	Query            string   `json:"query,omitempty"`
	RecordID         string   `json:"record_id,omitempty"`
	RequestID        string   `json:"request_id,omitempty"`
	AccessRequestID  string   `json:"access_request_id,omitempty"`
	BrowserSessionID string   `json:"browser_session_id,omitempty"`
	ExpectedURL      string   `json:"expected_url,omitempty"`
	Purpose          string   `json:"purpose,omitempty"`
	Fields           []string `json:"fields,omitempty"`
}

func IsTool(name string) bool {
	return name == "credentials.search" || name == "credentials.request_access" || name == "credentials.request_create" || name == "credentials.request_otp" || name == "credentials.request_status"
}
func DecodeToolArgs(name, raw string) (ToolArgs, error) {
	var a ToolArgs
	if !IsTool(name) || communicationcontrol.StrictJSON([]byte(raw), &a) != nil || len(a.Query) > 256 || len(a.Purpose) > 500 {
		return a, ErrVault
	}
	return a, nil
}
func Contracts() []*toolbrokerv1.ToolContract {
	var out []*toolbrokerv1.ToolContract
	for _, spec := range []struct{ name, description, required string }{
		{"credentials.search", "Find account records by alias or a login already supplied by the owner. Site hints do not restrict use. Returns metadata only; search is never permission to use a credential.", "[]"},
		{"credentials.request_access", "Ask the owner to use an existing credential for the shown browser site and purpose. No secret values are accepted. Use only the approved request ID as credential_ref.alias after owner response.", `["record_id","browser_session_id","expected_url","purpose","fields"]`},
		{"credentials.request_create", "Ask the owner to create an account record through the protected vault form. Never ask for passwords in ordinary chat. The owner may select an existing record instead.", `["browser_session_id","expected_url","purpose","fields"]`},
		{"credentials.request_otp", "Request a short-lived one-time code through the protected form for the current browser login. Include access_request_id from the previously approved password request to permit an owner-configured SMS relay. Never read a code from ordinary memory or ask for it in chat.", `["browser_session_id","expected_url","purpose"]`},
		{"credentials.request_status", "Read an existing credential request status. Returns only the request reference and permitted field names. Pending requests require owner input, never repeated polling.", `["request_id"]`},
	} {
		schema := `{"type":"object","required":` + spec.required + `,"properties":{"query":{"type":"string"},"record_id":{"type":"string"},"request_id":{"type":"string"},"access_request_id":{"type":"string"},"browser_session_id":{"type":"string"},"expected_url":{"type":"string"},"purpose":{"type":"string"},"fields":{"type":"array","items":{"type":"string","enum":["username","password"]}}},"additionalProperties":false}`
		out = append(out, &toolbrokerv1.ToolContract{ToolName: spec.name, Description: spec.description, ToolClass: "credential_control", RuntimeTarget: ControlTarget, InputSchemaJson: schema, OutputSchemaJson: `{"type":"object","properties":{"entries":{"type":"array","items":{"type":"object"}},"request":{"type":"object"}},"additionalProperties":false}`, Status: "active", ContractRevision: "1", RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}})
	}
	for _, c := range out {
		if c.ToolName == "credentials.search" || c.ToolName == "credentials.request_status" {
			c.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
		}
	}
	return out
}
func Contract(name string) *toolbrokerv1.ToolContract {
	for _, c := range Contracts() {
		if c.ToolName == name {
			return c
		}
	}
	return nil
}
