package credentialcontrol

import (
	"errors"
	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/externalauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

var ErrVault = errors.New("credential request unavailable or stale")

const ControlTarget = "broker://credential-vault-v1"

type ToolArgs struct {
	ExternalScope    *externalauth.Scope `json:"external_scope,omitempty"`
	SSOScope         *browsersso.Scope   `json:"sso_scope,omitempty"`
	LoginScope       *browserauth.Scope  `json:"login_scope,omitempty"`
	Query            string              `json:"query,omitempty"`
	RecordID         string              `json:"record_id,omitempty"`
	RequestID        string              `json:"request_id,omitempty"`
	AccessRequestID  string              `json:"access_request_id,omitempty"`
	BrowserSessionID string              `json:"browser_session_id,omitempty"`
	ExpectedURL      string              `json:"expected_url,omitempty"`
	Purpose          string              `json:"purpose,omitempty"`
	Fields           []string            `json:"fields,omitempty"`
}

func IsTool(name string) bool {
	return name == "credentials.search" || name == "credentials.request_access" || name == "credentials.request_create" || name == "credentials.request_otp" || name == "credentials.request_status" || name == "credentials.request_login" || name == browsersso.RequestTool || name == externalauth.RequestTool
}
func DecodeToolArgs(name, raw string) (ToolArgs, error) {
	var a ToolArgs
	if !IsTool(name) || communicationcontrol.StrictJSON([]byte(raw), &a) != nil || len(a.Query) > 256 || len(a.Purpose) > 500 {
		return a, ErrVault
	}
	if name == externalauth.RequestTool {
		r, e := externalauth.ParseRequest(raw)
		if e != nil {
			return a, ErrVault
		}
		a.ExternalScope = &r.Scope
		a.RecordID, a.Purpose = r.RecordID, r.Purpose
		a.BrowserSessionID, a.ExpectedURL = r.Scope.Browser, r.Scope.RPURL
	} else if a.ExternalScope != nil {
		return a, ErrVault
	}
	if name == "credentials.request_login" {
		if a.LoginScope == nil || a.LoginScope.Validate() != nil || a.RecordID == "" {
			return a, ErrVault
		}
		a.BrowserSessionID = a.LoginScope.Browser
		a.ExpectedURL = a.LoginScope.ExpectedURL
	} else if a.LoginScope != nil {
		return a, ErrVault
	}
	if name == browsersso.RequestTool {
		if a.SSOScope == nil || a.SSOScope.Validate() != nil || a.RecordID == "" {
			return a, ErrVault
		}
		a.BrowserSessionID = a.SSOScope.Browser
		a.ExpectedURL = a.SSOScope.RPURL
	} else if a.SSOScope != nil {
		return a, ErrVault
	}
	return a, nil
}
func Contracts(external ...bool) []*toolbrokerv1.ToolContract {
	var out []*toolbrokerv1.ToolContract
	specs := []struct{ name, description, required string }{
		{browsersso.RequestTool, "Ask the owner once for one popup SSO login from the exact observed RP document through the specified IdP, one native form POST and protected RP verification. The approved request is usable only by browser.sso_login with its scope digest; it grants no generic click/POST.", `["record_id","purpose","sso_scope"]`},
		{"credentials.search", "Find account records by alias or a login already supplied by the owner. Site hints do not restrict use. Returns metadata only; search is never permission to use a credential.", "[]"},
		{"credentials.request_access", "Ask the owner to use an existing credential for the shown browser site and purpose. No secret values are accepted. Use only the approved request ID as credential_ref.alias after owner response.", `["record_id","browser_session_id","expected_url","purpose","fields"]`},
		{"credentials.request_login", "Ask the owner once to fill and submit one exact ordinary login form using a saved account and verify access. Inspect browser forms first. After approval use browser.login with the returned request reference for both username/password. This grants no generic click or other POST.", `["record_id","purpose","login_scope"]`},
		{"credentials.request_create", "Ask the owner to create an account record through the protected vault form. Never ask for passwords in ordinary chat. The owner may select an existing record instead.", `["browser_session_id","expected_url","purpose","fields"]`},
		{"credentials.request_otp", "Request a short-lived one-time code through the protected form for the current browser login. Include access_request_id from the previously approved password request to permit an owner-configured SMS relay. Never read a code from ordinary memory or ask for it in chat.", `["browser_session_id","expected_url","purpose"]`},
		{"credentials.request_status", "Read an existing credential request status. Returns only the request reference and permitted field names. Pending requests require owner input, never repeated polling.", `["request_id"]`},
	}
	if len(external) == 1 && external[0] {
		specs = append(specs, struct{ name, description, required string }{externalauth.RequestTool, "Request one protected external Google sign-in in the owner's normal browser for this exact RP page, saved account and current task. The kernel waits for the same browser-bound callback and independently verified RP access, then resumes this Input. No password, token, private link or credential reference is returned. No polling, generic POST or user-tab credential injection.", `["record_id","purpose","external_scope"]`})
	}
	for _, spec := range specs {
		schema := `{"type":"object","required":` + spec.required + `,"properties":{"query":{"type":"string"},"record_id":{"type":"string"},"request_id":{"type":"string"},"access_request_id":{"type":"string"},"browser_session_id":{"type":"string"},"expected_url":{"type":"string"},"purpose":{"type":"string"},"fields":{"type":"array","items":{"type":"string","enum":["username","password"]}}},"additionalProperties":false}`
		if spec.name == "credentials.request_login" {
			schema = `{"type":"object","required":["record_id","purpose","login_scope"],"properties":{"record_id":{"type":"string"},"purpose":{"type":"string"},"login_scope":{"type":"object","required":["browser_session_id","expected_url","target_url","form_selector","username_selector","password_selector"],"properties":{"browser_session_id":{"type":"string"},"expected_url":{"type":"string"},"target_url":{"type":"string"},"form_selector":{"type":"string"},"username_selector":{"type":"string"},"password_selector":{"type":"string"}},"additionalProperties":false}},"additionalProperties":false}`
		}
		if spec.name == browsersso.RequestTool {
			schema = `{"type":"object","required":["record_id","purpose","sso_scope"],"properties":{"record_id":{"type":"string"},"purpose":{"type":"string"},"sso_scope":` + browsersso.ScopeSchema + `},"additionalProperties":false}`
		}
		if spec.name == externalauth.RequestTool {
			schema = `{"type":"object","required":["record_id","purpose","external_scope"],"properties":{"record_id":{"type":"string"},"purpose":{"type":"string"},"external_scope":` + externalauth.ScopeSchema + `},"additionalProperties":false}`
		}
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
	for _, c := range Contracts(true) {
		if c.ToolName == name {
			return c
		}
	}
	return nil
}
