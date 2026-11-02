package browsersso

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/browserpage"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/jsonstrict"
)

const InspectTool = "browser.sso_inspect"

// Inspection requests only a bounded GET-only popup observation. It contains no
// credential/consent reference and grants no submission or generic page access.
type Inspection struct {
	Browser   string          `json:"browser_session_id"`
	RPPage    browserpage.Ref `json:"rp_page"`
	RPURL     string          `json:"rp_url"`
	Trigger   string          `json:"trigger_selector"`
	IdPOrigin string          `json:"idp_origin"`
	VerifyURL string          `json:"verify_url"`
}

func (a Inspection) Validate() error {
	rp := Origin(a.RPURL)
	if !browserpage.ID(a.Browser) || !a.RPPage.Valid() || rp == "" || a.IdPOrigin == "" || a.IdPOrigin == rp || Origin(a.IdPOrigin+"/") != a.IdPOrigin || Origin(a.VerifyURL) != rp || a.VerifyURL == a.RPURL || strings.TrimSpace(a.Trigger) == "" || len(a.Trigger) > 1024 || strings.ContainsAny(a.Trigger, "\x00\r\n") {
		return ErrScope
	}
	return nil
}
func ParseInspection(raw string) (Inspection, error) {
	var a Inspection
	if len(raw) > 8192 || jsonstrict.Decode([]byte(raw), &a) != nil || a.Validate() != nil {
		return Inspection{}, ErrScope
	}
	return a, nil
}
func (a Inspection) Targets() []string { return []string{a.RPURL, a.IdPOrigin + "/", a.VerifyURL} }
func (a Inspection) Digest() string {
	raw, _ := json.Marshal(a)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// InspectedForm exports only canonical navigation/action paths and bounded
// positional selectors. It contains no text, field names/values, hidden CSRF,
// provider query, cookies, document body or private authorization material.
type InspectedForm struct {
	IdPURL    string `json:"idp_url"`
	TargetURL string `json:"target_url"`
	Form      string `json:"form_selector"`
	Username  string `json:"username_selector"`
	Password  string `json:"password_selector"`
}

func positional(s, kind string, cap int) bool {
	value, ok := strings.CutPrefix(s, kind+" >> nth=")
	if !ok {
		return false
	}
	index, err := strconv.Atoi(value)
	return err == nil && index >= 0 && index < cap && strconv.Itoa(index) == value
}
func (f InspectedForm) Scope(a Inspection) (Scope, error) {
	s := Scope{Browser: a.Browser, RPPage: a.RPPage, RPURL: a.RPURL, Trigger: a.Trigger, IdPURL: f.IdPURL, TargetURL: f.TargetURL, Form: f.Form, Username: f.Username, Password: f.Password, VerifyURL: a.VerifyURL}
	if a.Validate() != nil || s.Validate() != nil || Origin(f.IdPURL) != a.IdPOrigin || !positional(f.Form, "form", 8) || !positional(f.Username, "input", 128) || !positional(f.Password, "input", 128) {
		return Scope{}, ErrScope
	}
	return s, nil
}

func InspectContract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: InspectTool, ContractRevision: "1", ToolClass: "browser", RuntimeTarget: "tool-browser:9090", Description: "Inspect a native IdP login form in one fresh popup opened by an exact RP page. Choose the RP page and trigger from its current observation and the intended IdP origin. Returns value-free sso_scopes with actual form paths/selectors; choose the suitable scope for credentials.request_sso_login. GET/HEAD only, no fields filled or submitted, popup closed and network frozen. Does not support Google/JS/MFA OAuth; use the separate configured external login protocol for that.", InputSchemaJson: `{"type":"object","required":["browser_session_id","rp_page","rp_url","trigger_selector","idp_origin","verify_url"],"properties":{"browser_session_id":{"type":"string"},"rp_page":{"type":"object","required":["page_id","document_id","observation_version"],"properties":{"page_id":{"type":"string"},"document_id":{"type":"string"},"observation_version":{"type":"integer","minimum":1}},"additionalProperties":false},"rp_url":{"type":"string"},"trigger_selector":{"type":"string"},"idp_origin":{"type":"string"},"verify_url":{"type":"string"}},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object"}`, RiskLevel: "medium", Status: "active", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}}
}

// ValidInspection checks only private dispatch shape/caps. It is not consent.
func (d Dispatch) ValidInspection(a Inspection) bool {
	return a.Validate() == nil && d.validTargets(a.RPPage, a.RPURL, a.Targets()) && d.Digest == a.Digest()
}
