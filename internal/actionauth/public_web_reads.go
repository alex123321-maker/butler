package actionauth

import (
	"encoding/json"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// The operator can grant standing release consent for public page URLs/queries.
// First validate every destination through the existing public-network boundary.
// The exception never grants a write, credential use or extension execution.
func (s *Service) networkReleaseRequired(call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (bool, error) {
	required, err := NetworkReleaseRequired(call, contract)
	if err != nil || !required || !s.publicWebReads || len(call.GetCredentialRefs()) != 0 || contract.GetExtension() != nil || contract.GetConverter() != nil {
		return required, err
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil {
		return required, nil
	}
	// Custom headers/body may carry account or private payloads independently
	// of the page URL. They retain their normal exact release decision.
	for _, key := range []string{"headers", "body", "credential_ref", "credential_refs"} {
		if _, present := args[key]; present {
			return required, nil
		}
	}
	switch call.GetToolName() {
	case "browser.submit_get_form", "browser.follow_link":
		if contract.GetContractRevision() == "2" && !contract.GetSupportsCredentialRefs() && contract.GetEffectContract().GetRetrySafety() == "read_only" && contract.GetEffectContract().GetVerificationMethod() == BrowserGETVerification {
			return false, nil
		}
	case "web.search", "web.fetch", "web.fetch_batch", "web.extract", "http.download", "http.parse_html":
		return false, nil
	case "browser.read", "browser.open", "browser.navigate":
		// Revision 3 runtime navigation enforces the read-only network boundary.
		if contract.GetContractRevision() == "3" {
			return false, nil
		}
	case "http.request":
		method, _ := args["method"].(string)
		if method = strings.ToUpper(method); method == "" || method == "GET" || method == "HEAD" {
			return false, nil
		}
	}
	return required, nil
}
