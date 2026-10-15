package registry

import (
	"encoding/json"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
)

func (r *Registry) validateNetworkCap(call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) *toolbrokerv1.ToolError {
	classDomains, configured := r.classAllowedDomains[contract.GetToolClass()]
	if !configured {
		return nil
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil {
		return validationError("invalid network arguments")
	}
	var urls []string
	for _, key := range []string{"url", "expected_url"} {
		if value, ok := args[key].(string); ok && value != "" {
			urls = append(urls, value)
		}
	}
	if values, ok := args["urls"].([]any); ok {
		for _, value := range values {
			if raw, ok := value.(string); ok {
				urls = append(urls, raw)
			}
		}
	}
	for _, raw := range urls {
		// An empty intersection must never become Policy's unrestricted empty slice.
		_, classErr := (egress.Policy{AllowedDomains: classDomains}).ValidateURL(raw)
		_, contractErr := (egress.Policy{AllowedDomains: contract.GetAllowedDomains()}).ValidateURL(raw)
		if len(classDomains) == 0 || classErr != nil || contractErr != nil {
			return &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: "destination denied by operator network cap", Retryable: false}
		}
	}
	return nil
}
